package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/url"
	"os"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/internal/common/split"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/ohkinozomu/fuyuu-router/pkg/topics"
	"github.com/thanos-io/objstore"
	objstoreclient "github.com/thanos-io/objstore/client"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type processChPayload struct {
	requestPacket   *data.HTTPRequestPacket
	httpRequestData *data.HTTPRequestData
}

type mergeChPayload struct {
	requestPacket   *data.HTTPRequestPacket
	httpRequestData *data.HTTPRequestData
}

type server struct {
	client       *autopaho.ConnectionManager
	id           string
	proxyHost    string
	logger       *zap.Logger
	protocol     string
	encoder      *zstd.Encoder
	decoder      *zstd.Decoder
	commonConfig common.CommonConfigV2
	bucket       objstore.Bucket
	payloadCh    chan []byte
	mergeCh      chan mergeChPayload
	processCh    chan processChPayload
	merger       *split.Merger
}

func createWillMessage(c AgentConfig) *paho.WillMessage {
	teminatePacket := data.TerminatePacket{
		AgentId: c.ID,
		Labels:  c.Labels,
	}

	var terminatePayload []byte
	var err error
	switch c.CommonConfigV2.Networking.Format {
	case "json":
		terminatePayload, err = json.Marshal(&teminatePacket)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	case "protobuf":
		terminatePayload, err = proto.Marshal(&teminatePacket)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
	default:
		c.Logger.Fatal("Unknown format: " + c.CommonConfigV2.Networking.Format)
	}
	return &paho.WillMessage{
		Retain:  false,
		QoS:     0,
		Topic:   topics.TerminateTopic(),
		Payload: terminatePayload,
	}
}

func newServer(c AgentConfig) server {
	u, err := url.Parse(c.MQTTBroker)
	if err != nil {
		c.Logger.Fatal("Error parsing MQTT broker URL: " + err.Error())
	}

	payloadCh := make(chan []byte, 1000)
	mergeCh := make(chan mergeChPayload, 1000)
	processCh := make(chan processChPayload, 1000)

	var connectUsername string
	var connectPassword []byte
	if c.Username != "" && c.Password != "" {
		connectUsername = c.Username
		connectPassword = []byte(c.Password)
	}

	var tlsConfig tls.Config
	if c.CAFile != "" && c.Cert != "" && c.Key != "" {
		caCert, err := os.ReadFile(c.CAFile)
		if err != nil {
			c.Logger.Sugar().Fatalf("failed to read the CA cert file: %s", err)
		}

		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			c.Logger.Sugar().Fatalf("failed to append CA cert to the pool")
		}

		cert, err := tls.LoadX509KeyPair(c.Cert, c.Key)
		if err != nil {
			c.Logger.Sugar().Fatalf("failed to load the cert file: %s", err)
		}
		tlsConfig = tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{cert},
		}
	}

	cliCfg := autopaho.ClientConfig{
		TlsCfg:                        &tlsConfig,
		ServerUrls:                    []*url.URL{u},
		KeepAlive:                     20,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         60,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
			launchPacket := data.LaunchPacket{
				AgentId: c.ID,
				Labels:  c.Labels,
			}

			var launchPayload []byte
			switch c.CommonConfigV2.Networking.Format {
			case "json":
				launchPayload, err = json.Marshal(&launchPacket)
				if err != nil {
					c.Logger.Fatal(err.Error())
				}
			case "protobuf":
				launchPayload, err = proto.Marshal(&launchPacket)
				if err != nil {
					c.Logger.Fatal(err.Error())
				}
			default:
				c.Logger.Fatal("Unknown format: " + c.CommonConfigV2.Networking.Format)
			}

			_, err = cm.Publish(context.Background(), &paho.Publish{
				Topic: topics.LaunchTopic(),
				// Maybe it should be 1.
				QoS:     0,
				Payload: launchPayload,
			})
			if err != nil {
				c.Logger.Fatal(err.Error())
			}

			if _, err := cm.Subscribe(context.Background(), &paho.Subscribe{
				Subscriptions: []paho.SubscribeOptions{
					{Topic: topics.RequestTopic(c.ID), QoS: 0},
				},
			}); err != nil {
				c.Logger.Error("Error subscribing to MQTT topic: " + err.Error())
			}
			c.Logger.Info("Subscribed to MQTT topic")
		},
		OnConnectError: func(err error) {
			c.Logger.Error("Error connecting to MQTT broker: " + err.Error())
		},
		ClientConfig: paho.ClientConfig{
			ClientID: c.ID + "-" + uuid.New().String(),
			Router: paho.NewStandardRouterWithDefault(func(m *paho.Publish) {
				c.Logger.Debug("Received message")
				payloadCh <- m.Payload
			}),
			OnClientError: func(err error) {
				c.Logger.Error("Error from MQTT client: " + err.Error())
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				if d.Properties != nil {
					c.Logger.Error("server requested disconnect", zap.String("reason", d.Properties.ReasonString))
				} else {
					c.Logger.Error("server requested disconnect")
				}
			},
		},
		WillMessage:     createWillMessage(c),
		ConnectUsername: connectUsername,
		ConnectPassword: connectPassword,
	}

	cm, err := autopaho.NewConnection(context.Background(), cliCfg)
	if err != nil {
		c.Logger.Fatal(err.Error())
	}
	if err = cm.AwaitConnection(context.Background()); err != nil {
		c.Logger.Fatal(err.Error())
	}

	decoder, err := zstd.NewReader(
		nil,
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderLowmem(true),
	)
	if err != nil {
		c.Logger.Fatal(err.Error())
	}
	var encoder *zstd.Encoder
	if c.CommonConfigV2.Networking.Compress == "zstd" {
		encoder, err = zstd.NewWriter(
			nil,
			zstd.WithEncoderConcurrency(1),
			zstd.WithLowerEncoderMem(true),
			zstd.WithWindowSize(1<<20),
		)
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
		client:       cm,
		payloadCh:    payloadCh,
		mergeCh:      mergeCh,
		processCh:    processCh,
		logger:       c.Logger,
		commonConfig: c.CommonConfigV2,
		id:           c.ID,
		proxyHost:    c.ProxyHost,
		protocol:     c.Protocol,
		encoder:      encoder,
		decoder:      decoder,
		bucket:       bucket,
		merger:       split.NewMerger(),
	}
}

func sendHTTP1Request(proxyHost string, data *data.HTTPRequestData) ([]byte, int, http.Header, error) {
	var responseHeader http.Header

	url := "http://" + proxyHost + data.Path
	body := bytes.NewBuffer(data.Body.Body)
	req, err := http.NewRequest(data.Method, url, body)
	if err != nil {
		return nil, http.StatusInternalServerError, responseHeader, err
	}
	for key, values := range data.Headers.GetHeaders() {
		for _, value := range values.GetValues() {
			req.Header.Add(key, value)
		}
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, responseHeader, err
	}
	defer resp.Body.Close()
	responseBody := new(bytes.Buffer)
	_, err = responseBody.ReadFrom(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, responseHeader, err
	}
	return responseBody.Bytes(), resp.StatusCode, resp.Header, nil
}

func Start(c AgentConfig) {
	if c.Protocol != "http1" {
		c.Logger.Fatal("Unknown protocol: " + c.Protocol)
	}

	s := newServer(c)

	if c.CommonConfigV2.Telemetry.Enabled {
		exporter, err := prometheus.New()
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		meterName := "fuyuu-router-agent"
		provider := metric.NewMeterProvider(metric.WithReader(exporter))
		meter := provider.Meter(meterName)

		payloadChSize, err := meter.Float64ObservableGauge("buffered_payload_channel_size", api.WithDescription("The size of the buffered payloadCh"))
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
			n := float64(len(s.payloadCh))
			o.ObserveFloat64(payloadChSize, n)
			return nil
		}, payloadChSize)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		mergeChSize, err := meter.Float64ObservableGauge("buffered_merge_channel_size", api.WithDescription("The size of the buffered mergeCh"))
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
			n := float64(len(s.mergeCh))
			o.ObserveFloat64(mergeChSize, n)
			return nil
		}, mergeChSize)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		processChSize, err := meter.Float64ObservableGauge("buffered_process_channel_size", api.WithDescription("The size of the buffered processCh"))
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
			n := float64(len(s.processCh))
			o.ObserveFloat64(processChSize, n)
			return nil
		}, processChSize)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		go common.ServeMetrics(c.Logger)
	}

	for {
		select {
		case payload := <-s.payloadCh:
			go func() {
				requestPacket, err := data.Deserialize[*data.HTTPRequestPacket](payload, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Error("Error deserializing request packet", zap.Error(err))
					return
				}

				httpRequestData, err := data.DeserializeHTTPRequestData(requestPacket.HttpRequestData, s.commonConfig.Networking.Format, s.bucket)
				if err != nil {
					s.logger.Error("Error deserializing request data", zap.Error(err))
					return
				}
				if httpRequestData.Body.Type == "split" {
					s.logger.Debug("Received split message")
					s.mergeCh <- mergeChPayload{
						requestPacket:   requestPacket,
						httpRequestData: httpRequestData,
					}
				} else {
					processChPayload := processChPayload{
						requestPacket:   requestPacket,
						httpRequestData: httpRequestData,
					}
					s.processCh <- processChPayload
				}
			}()
		case mergeChPayload := <-s.mergeCh:
			go func() {
				combined, completed, err := split.Merge(s.merger, mergeChPayload.httpRequestData.Body.Body, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Info("Error merging message: " + err.Error())
					return
				}
				if completed {
					mergeChPayload.httpRequestData.Body.Body = combined
					s.processCh <- processChPayload(mergeChPayload)
					s.merger.DeleteChunk(mergeChPayload.requestPacket.RequestId)
				}
			}()
		case processChPayload := <-s.processCh:
			go func() {
				s.logger.Debug("Processing request")
				var responsePacket data.HTTPResponsePacket

				if s.protocol != "http1" {
					s.logger.Error("Unknown protocol: " + s.protocol)
					return
				}

				var err error
				if processChPayload.requestPacket.Compress == "zstd" && s.decoder != nil {
					processChPayload.httpRequestData.Body.Body, err = s.decoder.DecodeAll(processChPayload.httpRequestData.Body.Body, nil)
					if err != nil {
						s.logger.Error("Error decoding request body", zap.Error(err))
						return
					}
				}

				var responseData data.HTTPResponseData
				var objectName string
				httpResponse, statusCode, responseHeader, err := sendHTTP1Request(s.proxyHost, processChPayload.httpRequestData)
				if err != nil {
					s.logger.Error("Error sending HTTP request", zap.Error(err))
					protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)
					// For now, not apply the storage relay to the error
					responseData = data.HTTPResponseData{
						Body: &data.HTTPBody{
							Body: []byte(err.Error()),
							Type: "data",
						},
						StatusCode: http.StatusInternalServerError,
						Headers:    &protoHeaders,
					}
				} else {
					if s.commonConfig.Networking.Compress == "zstd" && s.encoder != nil {
						httpResponse = s.encoder.EncodeAll(httpResponse, nil)
					}

					if s.commonConfig.Networking.LargeDataPolicy == "split" && len(httpResponse) > s.commonConfig.Split.ChunkBytes {
						processFn := func(sequence int, b []byte) ([]byte, error) {
							body := data.HTTPBody{
								Body: b,
								Type: "split",
							}
							protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)
							responseData = data.HTTPResponseData{
								Body:       &body,
								StatusCode: int32(statusCode),
								Headers:    &protoHeaders,
							}

							b, err = data.Serialize(&responseData, s.commonConfig.Networking.Format)
							if err != nil {
								return nil, err
							}
							responsePacket = data.HTTPResponsePacket{
								RequestId:        processChPayload.requestPacket.RequestId,
								HttpResponseData: b,
								Compress:         s.commonConfig.Networking.Compress,
							}

							responsePayload, err := data.Serialize(&responsePacket, s.commonConfig.Networking.Format)
							if err != nil {
								return nil, err
							}
							return responsePayload, nil
						}

						sendFn := func(payload []byte) error {
							responseTopic := topics.ResponseTopic(s.id, processChPayload.requestPacket.RequestId)
							_, err = s.client.Publish(context.Background(), &paho.Publish{
								Topic:   responseTopic,
								QoS:     0,
								Payload: payload,
							})
							if err != nil {
								return err
							}
							return nil
						}

						err := split.Split(processChPayload.requestPacket.RequestId, httpResponse, s.commonConfig.Split.ChunkBytes, s.commonConfig.Networking.Format, processFn, sendFn)
						if err != nil {
							s.logger.Error("Error splitting message", zap.Error(err))
							return
						}
					} else {
						if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(httpResponse) > s.commonConfig.StorageRelay.ThresholdBytes {
							objectName = common.ResponseObjectName(s.id, processChPayload.requestPacket.RequestId)
							err := s.bucket.Upload(context.Background(), objectName, bytes.NewReader(httpResponse))
							if err != nil {
								s.logger.Error("Error uploading object to object storage", zap.Error(err))
								return
							}
						}
						protoHeaders := data.HTTPHeaderToProtoHeaders(responseHeader)

						var body data.HTTPBody
						if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(httpResponse) > s.commonConfig.StorageRelay.ThresholdBytes {
							s.logger.Debug("Using storage relay")
							s.logger.Debug("Object name: " + objectName)
							body = data.HTTPBody{
								Body: []byte(objectName),
								Type: "storage_relay",
							}
						} else {
							body = data.HTTPBody{
								Body: []byte(httpResponse),
								Type: "data",
							}
						}

						responseData = data.HTTPResponseData{
							Body:       &body,
							StatusCode: int32(statusCode),
							Headers:    &protoHeaders,
						}
					}
				}
				b, err := data.Serialize(&responseData, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Error("Error serializing response data", zap.Error(err))
					return
				}
				responsePacket = data.HTTPResponsePacket{
					RequestId:        processChPayload.requestPacket.RequestId,
					HttpResponseData: b,
					Compress:         s.commonConfig.Networking.Compress,
				}

				responseTopic := topics.ResponseTopic(s.id, processChPayload.requestPacket.RequestId)

				responsePayload, err := data.Serialize(&responsePacket, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Error("Error serializing response packet", zap.Error(err))
					return
				}

				s.logger.Debug("Publishing response")
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
