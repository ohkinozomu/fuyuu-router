package agent

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/klauspost/compress/zstd"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/ohkinozomu/fuyuu-router/pkg/topics"
	"github.com/thanos-io/objstore"
	objstoreclient "github.com/thanos-io/objstore/client"
	"go.uber.org/zap"
)

type Router struct {
	messageChan  chan string
	client       *paho.Client
	id           string
	proxyHost    string
	logger       *zap.Logger
	protocol     string
	encoder      *zstd.Encoder
	decoder      *zstd.Decoder
	commonConfig common.CommonConfigV2
	bucket       objstore.Bucket
}

var _ paho.Router = (*Router)(nil)

func NewRouter(messageChan chan string, c AgentConfig) *Router {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := common.TCPConnect(ctx, c.CommonConfig)
	if err != nil {
		c.Logger.Fatal("Error: " + err.Error())
	}
	clientConfig := paho.ClientConfig{
		Conn: conn,
	}
	client := paho.NewClient(clientConfig)

	_, err = client.Connect(context.Background(), common.MQTTConnect(c.CommonConfig))
	if err != nil {
		c.Logger.Fatal("Error connecting to MQTT broker: " + err.Error())
	}

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

	return &Router{
		messageChan:  messageChan,
		client:       client,
		id:           c.ID,
		proxyHost:    c.ProxyHost,
		logger:       c.Logger,
		protocol:     c.Protocol,
		encoder:      encoder,
		decoder:      decoder,
		commonConfig: c.CommonConfigV2,
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

func (r *Router) Route(p *packets.Publish) {
	requestPacket, err := data.DeserializeRequestPacket(p.Payload, r.commonConfig.Networking.Format)
	if err != nil {
		r.logger.Error("Error deserializing request packet", zap.Error(err))
		return
	}

	var responsePacket data.HTTPResponsePacket
	httpRequestData, err := data.DeserializeHTTPRequestData(requestPacket.HttpRequestData, requestPacket.Compress, r.commonConfig.Networking.Format, r.decoder, r.bucket)
	if err != nil {
		r.logger.Error("Error deserializing request data", zap.Error(err))
		return
	}

	if r.protocol == "http1" {
		var responseData data.HTTPResponseData
		var objectName string
		httpResponse, statusCode, responseHeader, err := sendHTTP1Request(r.proxyHost, httpRequestData)
		if err != nil {
			r.logger.Error("Error sending HTTP request", zap.Error(err))
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
			if r.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(httpResponse) > r.commonConfig.StorageRelay.ThresholdBytes {
				objectName = r.id + "/" + requestPacket.RequestId + "/response"
				err := r.bucket.Upload(context.Background(), objectName, strings.NewReader(httpResponse))
				if err != nil {
					r.logger.Error("Error uploading object to object storage", zap.Error(err))
					return
				}
			}
			protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)

			var body data.HTTPBody
			if r.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(httpResponse) > r.commonConfig.StorageRelay.ThresholdBytes {
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
		b, err := data.SerializeHTTPResponseData(&responseData, r.commonConfig.Networking.Format, r.encoder)
		if err != nil {
			r.logger.Error("Error serializing response data", zap.Error(err))
			return
		}
		responsePacket = data.HTTPResponsePacket{
			RequestId:        requestPacket.RequestId,
			HttpResponseData: b,
			Compress:         r.commonConfig.Networking.Compress,
		}
	} else {
		r.logger.Error("Unknown protocol: " + r.protocol)
		return
	}

	responseTopic := topics.ResponseTopic(r.id, requestPacket.RequestId)

	responsePayload, err := data.SerializeResponsePacket(&responsePacket, r.commonConfig.Networking.Format)
	if err != nil {
		r.logger.Error("Error serializing response packet", zap.Error(err))
		return
	}
	_, err = r.client.Publish(context.Background(), &paho.Publish{
		Topic:   responseTopic,
		QoS:     0,
		Payload: responsePayload,
	})
	if err != nil {
		r.logger.Error("Error publishing response", zap.Error(err))
		return
	}
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(paho.Logger) {}
