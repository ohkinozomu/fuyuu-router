package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/klauspost/compress/zstd"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/ohkinozomu/fuyuu-router/pkg/topics"
	"github.com/thanos-io/objstore"
	objstoreclient "github.com/thanos-io/objstore/client"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type server struct {
	client       *paho.Client
	id           string
	proxyHost    string
	logger       *zap.Logger
	protocol     string
	encoder      *zstd.Encoder
	decoder      *zstd.Decoder
	commonConfig common.CommonConfigV2
	bucket       objstore.Bucket
	payloadCh    chan []byte
}

func newServer(c AgentConfig) server {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := common.TCPConnect(ctx, c.CommonConfig)
	if err != nil {
		c.Logger.Fatal("Error: " + err.Error())
	}
	payloadCh := make(chan []byte)
	clientConfig := paho.ClientConfig{
		Conn:   conn,
		Router: NewRouter(payloadCh, c),
	}
	client := paho.NewClient(clientConfig)

	var encoder *zstd.Encoder
	var decoder *zstd.Decoder
	if c.CommonConfigV2.Networking.Compress == "zstd" {
		encoder, err = zstd.NewWriter(nil)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		decoder, err = zstd.NewReader(nil)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	}

	var bucket objstore.Bucket
	if c.CommonConfigV2.Networking.LargeDataPolicy == "storage_relay" {
		objstoreConf, err := os.ReadFile(c.CommonConfigV2.StorageRelay.ObjstoreFile)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		bucket, err = objstoreclient.NewBucket(common.NewZapToGoKitAdapter(c.Logger), objstoreConf, "fuyuu-router-hub")
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	}

	return server{
		client:       client,
		payloadCh:    payloadCh,
		logger:       c.Logger,
		commonConfig: c.CommonConfigV2,
		id:           c.ID,
		proxyHost:    c.ProxyHost,
		protocol:     c.Protocol,
		encoder:      encoder,
		decoder:      decoder,
		bucket:       bucket,
	}
}

func sendHTTP1Request(proxyHost string, data *data.HTTPRequestData) (string, int, http.Header, error) {
	var responseHeader http.Header

	url := "http://" + proxyHost + data.Path
	body := bytes.NewBufferString(data.Body.Body)
	req, err := http.NewRequest(data.Method, url, body)
	if err != nil {
		return "", http.StatusInternalServerError, responseHeader, err
	}
	for key, values := range data.Headers.GetHeaders() {
		for _, value := range values.GetValues() {
			req.Header.Add(key, value)
		}
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", http.StatusInternalServerError, responseHeader, err
	}
	defer resp.Body.Close()
	responseBody := new(bytes.Buffer)
	_, err = responseBody.ReadFrom(resp.Body)
	if err != nil {
		return "", resp.StatusCode, responseHeader, err
	}
	return responseBody.String(), resp.StatusCode, resp.Header, nil
}

func Start(c AgentConfig) {
	if c.Protocol != "http1" {
		c.Logger.Fatal("Unknown protocol: " + c.Protocol)
	}

	s := newServer(c)
	connect := common.MQTTConnect(c.CommonConfig)
	teminatePacket := data.TerminatePacket{
		AgentId: c.ID,
		Labels:  c.Labels,
	}

	var terminatePayload []byte
	var err error
	if c.CommonConfigV2.Networking.Format == "json" {
		terminatePayload, err = json.Marshal(&teminatePacket)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	} else if c.CommonConfigV2.Networking.Format == "protobuf" {
		terminatePayload, err = proto.Marshal(&teminatePacket)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	} else {
		c.Logger.Fatal("Unknown format: " + c.CommonConfigV2.Networking.Format)
	}
	if err != nil {
		c.Logger.Fatal(err.Error())
	}

	connect.WillMessage = &paho.WillMessage{
		Retain:  false,
		QoS:     0,
		Topic:   topics.TerminateTopic(),
		Payload: terminatePayload,
	}
	_, err = s.client.Connect(context.Background(), connect)
	if err != nil {
		c.Logger.Fatal("Error connecting to MQTT broker: " + err.Error())
	}

	launchPacket := data.LaunchPacket{
		AgentId: c.ID,
		Labels:  c.Labels,
	}

	var launchPayload []byte
	if c.CommonConfigV2.Networking.Format == "json" {
		launchPayload, err = json.Marshal(&launchPacket)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	} else if c.CommonConfigV2.Networking.Format == "protobuf" {
		launchPayload, err = proto.Marshal(&launchPacket)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	} else {
		c.Logger.Fatal("Unknown format: " + c.CommonConfigV2.Networking.Format)
	}

	_, err = s.client.Publish(context.Background(), &paho.Publish{
		Topic: topics.LaunchTopic(),
		// Maybe it should be 1.
		QoS:     0,
		Payload: launchPayload,
	})
	if err != nil {
		c.Logger.Fatal(err.Error())
	}

	_, err = s.client.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{
				Topic: topics.RequestTopic(c.ID),
				QoS:   0,
			},
		},
	})
	if err != nil {
		c.Logger.Fatal(err.Error())
	}
	for {
		select {
		case payload := <-s.payloadCh:
			go func() {
				requestPacket, err := data.DeserializeRequestPacket(payload, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Error("Error deserializing request packet", zap.Error(err))
					return
				}

				var responsePacket data.HTTPResponsePacket
				httpRequestData, err := data.DeserializeHTTPRequestData(requestPacket.HttpRequestData, requestPacket.Compress, s.commonConfig.Networking.Format, s.decoder, s.bucket)
				if err != nil {
					s.logger.Error("Error deserializing request data", zap.Error(err))
					return
				}

				if s.protocol == "http1" {
					var responseData data.HTTPResponseData
					var objectName string
					httpResponse, statusCode, responseHeader, err := sendHTTP1Request(s.proxyHost, httpRequestData)
					if err != nil {
						s.logger.Error("Error sending HTTP request", zap.Error(err))
						protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)
						// For now, not apply the storage relay to the error
						responseData = data.HTTPResponseData{
							Body: &data.HTTPBody{
								Body: err.Error(),
								Type: "data",
							},
							StatusCode: http.StatusInternalServerError,
							Headers:    &protoHeaders,
						}
					} else {
						if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(httpResponse) > s.commonConfig.StorageRelay.ThresholdBytes {
							objectName = s.id + "/" + requestPacket.RequestId + "/response"
							err := s.bucket.Upload(context.Background(), objectName, strings.NewReader(httpResponse))
							if err != nil {
								s.logger.Error("Error uploading object to object storage", zap.Error(err))
								return
							}
						}
						protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)

						var body data.HTTPBody
						if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(httpResponse) > s.commonConfig.StorageRelay.ThresholdBytes {
							body = data.HTTPBody{
								Body: objectName,
								Type: "storage_relay",
							}
						} else {
							body = data.HTTPBody{
								Body: httpResponse,
								Type: "data",
							}
						}

						responseData = data.HTTPResponseData{
							Body:       &body,
							StatusCode: int32(statusCode),
							Headers:    &protoHeaders,
						}
					}
					b, err := data.SerializeHTTPResponseData(&responseData, s.commonConfig.Networking.Format, s.encoder)
					if err != nil {
						s.logger.Error("Error serializing response data", zap.Error(err))
						return
					}
					responsePacket = data.HTTPResponsePacket{
						RequestId:        requestPacket.RequestId,
						HttpResponseData: b,
						Compress:         s.commonConfig.Networking.Compress,
					}
				} else {
					s.logger.Error("Unknown protocol: " + s.protocol)
					return
				}

				responseTopic := topics.ResponseTopic(s.id, requestPacket.RequestId)

				responsePayload, err := data.SerializeResponsePacket(&responsePacket, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Error("Error serializing response packet", zap.Error(err))
					return
				}

				_, err = s.client.Publish(context.Background(), &paho.Publish{
					Topic:   responseTopic,
					QoS:     0,
					Payload: responsePayload,
				})
				if err != nil {
					s.logger.Error("Error publishing response", zap.Error(err))
					return
				}
			}()
		}
	}
}
