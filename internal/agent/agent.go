package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
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

func sendHTTP1Request(proxyHost string, data *data.HTTPRequestData) (*http.Response, error) {
	var response *http.Response

	url := "http://" + proxyHost + data.Path
	body := bytes.NewBuffer(data.Body.Body)
	req, err := http.NewRequest(data.Method, url, body)
	if err != nil {
		return nil, err
	}
	for key, values := range data.Headers.GetHeaders() {
		for _, value := range values.GetValues() {
			req.Header.Add(key, value)
		}
	}
	client := &http.Client{}
	response, err = client.Do(req)
	if err != nil {
		return response, err
	}
	return response, nil
}

func (s *server) sendOnce(requestID string, isLast bool, sequence int32, d []byte, r http.Response) error {
	httpBodyChunk := data.HTTPBodyChunk{
		RequestId: requestID,
		IsLast:    isLast,
		Sequence:  int32(sequence),
		Data:      d,
	}
	b, err := data.SerializeHTTPBodyChunk(&httpBodyChunk, s.commonConfig.Networking.Format)
	if err != nil {
		return err
	}
	body := data.HTTPBody{
		Body: b,
		Type: "split",
	}
	protoHeaders := data.HTTPHeaderToProtoHeaders(r.Header)
	responseData := data.HTTPResponseData{
		Body:       &body,
		StatusCode: int32(r.StatusCode),
		Headers:    &protoHeaders,
	}
	sendErr := s.sendResponseData(&responseData, requestID)
	if sendErr != nil {
		return sendErr
	}
	return nil
}

func (s *server) sendSplitData(requestID string, httpResponse *http.Response) error {
	buffer := make([]byte, s.commonConfig.Split.ChunkBytes)
	sequence := 1
	isLast := false
	previousData := []byte{}
	for {
		n, readErr := io.ReadFull(httpResponse.Body, buffer)

		if n > 0 {
			s.logger.Debug(string(buffer[:n]))
			previousData = make([]byte, n)
			copy(previousData, buffer[:n])
		}

		if readErr != nil {
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				isLast = true
				if len(previousData) > 0 {
					err := s.sendOnce(requestID, isLast, int32(sequence), previousData, *httpResponse)
					if err != nil {
						return err
					}
				}
				// The request is smaller than a chunk and only one packet is sent.
				if len(previousData) == 0 {
					err := s.sendOnce(requestID, isLast, int32(sequence), buffer[:n], *httpResponse)
					if err != nil {
						return err
					}
				}
				break
			} else {
				s.logger.Error("Error reading request body", zap.Error(readErr))
				return readErr
			}
		} else {
			if len(previousData) > 0 {
				err := s.sendOnce(requestID, false, int32(sequence), previousData, *httpResponse)
				if err != nil {
					return err
				}
			}
		}
		sequence++
	}

	return nil
}

func (s *server) sendResponseData(responseData *data.HTTPResponseData, requestID string) error {
	b, err := data.SerializeHTTPResponseData(responseData, s.commonConfig.Networking.Format)
	if err != nil {
		return err
	}

	if s.commonConfig.Networking.Compress == "zstd" && s.encoder != nil {
		b = s.encoder.EncodeAll(b, nil)
	}

	responsePacket := data.HTTPResponsePacket{
		RequestId:        requestID,
		HttpResponseData: b,
		Compress:         s.commonConfig.Networking.Compress,
	}

	responseTopic := topics.ResponseTopic(s.id, requestID)

	responsePayload, err := data.SerializeResponsePacket(&responsePacket, s.commonConfig.Networking.Format)
	if err != nil {
		return err
	}

	s.logger.Debug("Publishing response")
	_, err = s.client.Publish(context.Background(), &paho.Publish{
		Topic:   responseTopic,
		QoS:     1,
		Payload: responsePayload,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *server) sendUnsplitData(requestID string, httpResponse *http.Response) error {
	s.logger.Debug("Reading response body")
	responseBody := new(bytes.Buffer)
	_, err := responseBody.ReadFrom(httpResponse.Body)
	if err != nil {
		return err
	}

	var objectName string
	if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(responseBody.Bytes()) > s.commonConfig.StorageRelay.ThresholdBytes {
		objectName = common.ResponseObjectName(s.id, requestID)
		s.logger.Debug("Object name: " + objectName)
		s.logger.Debug("Uploading object")
		err := s.bucket.Upload(context.Background(), objectName, bytes.NewReader(responseBody.Bytes()))
		if err != nil {
			return err
		}
	}
	protoHeaders := data.HTTPHeaderToProtoHeaders(httpResponse.Header)

	var body data.HTTPBody
	if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && len(responseBody.Bytes()) > s.commonConfig.StorageRelay.ThresholdBytes {
		s.logger.Debug("Using storage relay")
		s.logger.Debug("Object name: " + objectName)
		body = data.HTTPBody{
			Body: []byte(objectName),
			Type: "storage_relay",
		}
	} else {
		body = data.HTTPBody{
			Body: []byte(responseBody.Bytes()),
			Type: "data",
		}
	}

	responseData := data.HTTPResponseData{
		Body:       &body,
		StatusCode: int32(httpResponse.StatusCode),
		Headers:    &protoHeaders,
	}
	err = s.sendResponseData(&responseData, requestID)
	if err != nil {
		return err
	}
	return nil
}

func (s *server) handleErr(requestID string, err error) {
	s.logger.Error("Error sending HTTP request", zap.Error(err))
	// For now, not apply the storage relay to the error
	responseData := data.HTTPResponseData{
		Body: &data.HTTPBody{
			Body: []byte(err.Error()),
			Type: "data",
		},
		StatusCode: http.StatusInternalServerError,
	}
	err = s.sendResponseData(&responseData, requestID)
	if err != nil {
		s.logger.Error("Error sending response data", zap.Error(err))
		return
	}
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
				requestPacket, err := data.DeserializeRequestPacket(payload, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Error("Error deserializing request packet", zap.Error(err))
					return
				}

				if requestPacket.Compress == "zstd" && s.decoder != nil {
					requestPacket.HttpRequestData, err = s.decoder.DecodeAll(requestPacket.HttpRequestData, nil)
					if err != nil {
						s.logger.Error("Error decoding request body", zap.Error(err))
						return
					}
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
				s.logger.Debug("Merging message")
				combined, completed, err := split.Merge(s.merger, mergeChPayload.httpRequestData.Body.Body, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Info("Error merging message: " + err.Error())
					return
				}
				if completed {
					s.logger.Debug("combined: " + string(combined))
					mergeChPayload.httpRequestData.Body.Body = combined
					s.processCh <- processChPayload(mergeChPayload)
					s.merger.DeleteChunk(mergeChPayload.requestPacket.RequestId)
				}
			}()
		case processChPayload := <-s.processCh:
			go func() {
				s.logger.Debug("Processing request")

				if s.protocol != "http1" {
					s.logger.Error("Unknown protocol: " + s.protocol)
					return
				}

				var err error
				httpResponse, err := sendHTTP1Request(s.proxyHost, processChPayload.httpRequestData)
				if err != nil {
					s.handleErr(processChPayload.requestPacket.RequestId, err)
					return
				}
				defer httpResponse.Body.Close()

				if s.commonConfig.Networking.LargeDataPolicy == "split" {
					err = s.sendSplitData(processChPayload.requestPacket.RequestId, httpResponse)
					if err != nil {
						s.handleErr(processChPayload.requestPacket.RequestId, err)
						return
					}
				} else {
					err = s.sendUnsplitData(processChPayload.requestPacket.RequestId, httpResponse)
					if err != nil {
						s.handleErr(processChPayload.requestPacket.RequestId, err)
						return
					}
				}
			}()
		}
	}
}
