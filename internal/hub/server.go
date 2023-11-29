package hub

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/klauspost/compress/zstd"
	"github.com/mustafaturan/bus/v3"
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
)

type mergeChPayload struct {
	responsePacket   *data.HTTPResponsePacket
	httpResponseData *data.HTTPResponseData
}

type busChPayload struct {
	responsePacket   *data.HTTPResponsePacket
	httpResponseData *data.HTTPResponseData
}

type server struct {
	client                     *autopaho.ConnectionManager
	bus                        *bus.Bus
	logger                     *zap.Logger
	encoder                    *zstd.Encoder
	decoder                    *zstd.Decoder
	commonConfig               common.CommonConfigV2
	bucket                     objstore.Bucket
	merger                     *split.Merger
	payloadCh                  chan []byte
	mergeCh                    chan mergeChPayload
	busCh                      chan busChPayload
	httpRequestDurationSeconds api.Float64Histogram
}

func newClient(c HubConfig, payloadCh chan []byte) *autopaho.ConnectionManager {
	u, err := url.Parse(c.MQTTBroker)
	if err != nil {
		c.Logger.Fatal("Error parsing MQTT broker URL: " + err.Error())
	}

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
			c.Logger.Info("Connected to MQTT broker")
		},
		OnConnectError: func(err error) {
			c.Logger.Error("Error connecting to MQTT broker: " + err.Error())
		},
		ClientConfig: paho.ClientConfig{
			ClientID: "fuyuu-router-hub" + "-" + uuid.New().String(),
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
	return cm
}

func newServer(c HubConfig) server {
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

	payloadCh := make(chan []byte, 1000)
	mergeCh := make(chan mergeChPayload, 1000)
	busCh := make(chan busChPayload, 1000)
	client := newClient(c, payloadCh)

	var httpRequestDurationSeconds api.Float64Histogram
	if c.CommonConfigV2.Telemetry.Enabled {
		exporter, err := prometheus.New()
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		meterName := "fuyuu-router-hub"
		provider := metric.NewMeterProvider(metric.WithReader(exporter))
		meter := provider.Meter(meterName)

		payloadChSize, err := meter.Float64ObservableGauge("buffered_payload_channel_size", api.WithDescription("The size of the buffered payloadCh"))
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
			n := float64(len(payloadCh))
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
			n := float64(len(mergeCh))
			o.ObserveFloat64(mergeChSize, n)
			return nil
		}, mergeChSize)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		busChSize, err := meter.Float64ObservableGauge("buffered_bus_channel_size", api.WithDescription("The size of the buffered busCh"))
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
			n := float64(len(busCh))
			o.ObserveFloat64(busChSize, n)
			return nil
		}, busChSize)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		httpRequestDurationSeconds, err = meter.Float64Histogram(
			"http_request_duration_seconds",
			api.WithDescription("The HTTP request latencies in seconds."),
		)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

		go common.ServeMetrics(c.Logger)
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
		client:                     client,
		bus:                        newBus(),
		logger:                     c.Logger,
		encoder:                    encoder,
		decoder:                    decoder,
		commonConfig:               c.CommonConfigV2,
		bucket:                     bucket,
		merger:                     split.NewMerger(),
		payloadCh:                  payloadCh,
		mergeCh:                    mergeCh,
		busCh:                      busCh,
		httpRequestDurationSeconds: httpRequestDurationSeconds,
	}
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	var start time.Time
	if s.commonConfig.Telemetry.Enabled {
		start = time.Now()
	}

	s.logger.Debug("Handling request...")

	fuyuuRouterIDs := r.Header.Get("FuyuuRouter-IDs")
	if fuyuuRouterIDs == "" {
		w.Write([]byte("FuyuuRouter-IDs header is required"))
		if s.commonConfig.Telemetry.Enabled {
			elapsed := time.Since(start)
			s.logger.Debug("Request took " + elapsed.String())
			s.httpRequestDurationSeconds.Record(context.Background(), elapsed.Seconds())
		}
		return
	}
	agentIDs := strings.Split(fuyuuRouterIDs, ",")
	// Load Balancing
	// Now, select randomly
	agentID := agentIDs[rand.Intn(len(agentIDs))]
	s.logger.Debug("Selected agent: " + agentID)

	timeoutDuration := 60 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeoutDuration)
	defer cancel()

	go func() {
		<-r.Context().Done()
		cancel()
	}()

	uuid := uuid.New().String()

	var objectName string
	if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && r.ContentLength > int64(s.commonConfig.StorageRelay.ThresholdBytes) {
		objectName = common.RequestObjectName(agentID, uuid)
		s.logger.Debug("Uploading object to object storage...")
		err := s.bucket.Upload(context.Background(), objectName, r.Body)
		if err != nil {
			s.logger.Error("Error uploading object to object storage", zap.Error(err))
			return
		}
		if s.commonConfig.StorageRelay.Deletion {
			defer func() {
				err = s.bucket.Delete(context.Background(), common.RequestObjectName(agentID, uuid))
				if err != nil {
					s.logger.Error("Error deleting object from object storage", zap.Error(err))
				}
				err = s.bucket.Delete(context.Background(), common.ResponseObjectName(agentID, uuid))
				if err != nil {
					s.logger.Error("Error deleting object from object storage", zap.Error(err))
				}
			}()
		}
	}

	topic := topics.ResponseTopic(agentID, uuid)

	s.logger.Debug("Subscribing to response topic...")
	_, err := s.client.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{
				Topic: topic,
				QoS:   0,
			},
		},
	})
	if err != nil {
		s.logger.Error("Error subscribing to response topic", zap.Error(err))
		return
	}
	defer s.client.Unsubscribe(ctx, &paho.Unsubscribe{
		Topics: []string{topic},
	})

	dataCh := make(chan *data.HTTPResponseData)
	s.bus.RegisterTopics(uuid)
	defer s.bus.DeregisterTopics(uuid)

	handler := bus.Handler{
		Handle: func(ctx context.Context, e bus.Event) {
			s.logger.Debug("Received message from bus")
			dataCh <- e.Data.(*data.HTTPResponseData)
		},
		Matcher: uuid,
	}
	s.bus.RegisterHandler(uuid, handler)
	defer s.bus.DeregisterHandler(uuid)

	var bodyBytes []byte
	var body data.HTTPBody

	if s.commonConfig.Networking.LargeDataPolicy == "split" && r.ContentLength > int64(s.commonConfig.Split.ChunkBytes) {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("Error reading request body", zap.Error(err))
			return
		}

		processFn := func(sequence int, b []byte) ([]byte, error) {
			body = data.HTTPBody{
				Body: b,
				Type: "split",
			}
			protoHeaders := data.HTTPHeaderToProtoHeaders(r.Header)
			requestData := data.HTTPRequestData{
				Method:  r.Method,
				Path:    r.URL.Path,
				Headers: &protoHeaders,
				Body:    &body,
			}

			b, err = data.Serialize(&requestData, s.commonConfig.Networking.Format)
			if err != nil {
				return nil, err
			}

			requestPacket := data.HTTPRequestPacket{
				RequestId:       uuid,
				HttpRequestData: b,
				Compress:        s.commonConfig.Networking.Compress,
			}
			requestPayload, err := data.Serialize(&requestPacket, s.commonConfig.Networking.Format)
			if err != nil {
				return nil, err
			}
			if s.encoder != nil {
				requestPayload = s.encoder.EncodeAll(requestPayload, nil)
			}
			return requestPayload, nil
		}

		sendFn := func(payload []byte) error {
			requestTopic := topics.RequestTopic(agentID)
			_, err = s.client.Publish(context.Background(), &paho.Publish{
				Payload: payload,
				Topic:   requestTopic,
			})
			if err != nil {
				return err
			}
			return nil
		}

		err := split.Split(uuid, bodyBytes, s.commonConfig.Split.ChunkBytes, s.commonConfig.Networking.Format, processFn, sendFn)
		if err != nil {
			s.logger.Error("Error splitting message", zap.Error(err))
			return
		}
	} else {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("Error reading request body", zap.Error(err))
			return
		}

		if s.encoder != nil {
			bodyBytes = s.encoder.EncodeAll(bodyBytes, nil)
		}

		if s.commonConfig.Networking.LargeDataPolicy == "storage_relay" && r.ContentLength > int64(s.commonConfig.StorageRelay.ThresholdBytes) {
			body = data.HTTPBody{
				Body: []byte(objectName),
				Type: "storage_relay",
			}
		} else {
			body = data.HTTPBody{
				Body: bodyBytes,
				Type: "data",
			}
		}
		protoHeaders := data.HTTPHeaderToProtoHeaders(r.Header)
		requestData := data.HTTPRequestData{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: &protoHeaders,
			Body:    &body,
		}

		b, err := data.Serialize(&requestData, s.commonConfig.Networking.Format)
		if err != nil {
			s.logger.Error("Error serializing request data", zap.Error(err))
			return
		}

		requestPacket := data.HTTPRequestPacket{
			RequestId:       uuid,
			HttpRequestData: b,
			Compress:        s.commonConfig.Networking.Compress,
		}

		requestPayload, err := data.Serialize(&requestPacket, s.commonConfig.Networking.Format)
		if err != nil {
			s.logger.Error("Error serializing request packet", zap.Error(err))
			return
		}

		requestTopic := topics.RequestTopic(agentID)
		s.logger.Debug("Publishing request...")
		_, err = s.client.Publish(context.Background(), &paho.Publish{
			Payload: requestPayload,
			Topic:   requestTopic,
		})
		if err != nil {
			s.logger.Error("Error publishing request", zap.Error(err))
			return
		}
	}

	select {
	case httpResponseData := <-dataCh:
		s.logger.Debug("Writing response...")
		w.WriteHeader(int(httpResponseData.StatusCode))
		w.Write([]byte(httpResponseData.Body.Body))
		if s.commonConfig.Telemetry.Enabled {
			elapsed := time.Since(start)
			s.logger.Debug("Request took " + elapsed.String())
			s.httpRequestDurationSeconds.Record(context.Background(), elapsed.Seconds())
		}
		return
	case <-ctx.Done():
		s.logger.Debug("Request timed out...")
		if ctx.Err() == context.DeadlineExceeded {
			w.WriteHeader(http.StatusRequestTimeout)
			w.Write([]byte("The request timed out."))
			if s.commonConfig.Telemetry.Enabled {
				elapsed := time.Since(start)
				s.logger.Debug("Request took " + elapsed.String())
				s.httpRequestDurationSeconds.Record(context.Background(), elapsed.Seconds())
			}
			return
		}
	}
}

func Start(c HubConfig) {
	s := newServer(c)

	if c.Protocol != "http1" {
		c.Logger.Fatal("Unknown protocol: " + c.Protocol)
	}
	s.startHTTP1(c)
}

func (s *server) startHTTP1(c HubConfig) {
	r := mux.NewRouter()
	r.HandleFunc("/{id:.*}", s.handleRequest).Methods(
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodOptions,
		http.MethodHead,
		http.MethodConnect,
		http.MethodTrace,
	)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		c.Logger.Info("Starting server...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.Logger.Fatal("ListenAndServe error: %v", zap.Error(err))
		}
	}()

	go func() {
		for {
			select {
			case payload := <-s.payloadCh:
				go func() {
					s.logger.Debug("Received message")
					httpResponsePacket, err := data.Deserialize[*data.HTTPResponsePacket](payload, s.commonConfig.Networking.Format)
					if err != nil {
						s.logger.Info("Error deserializing response packet: " + err.Error())
						return
					}

					httpResponseData, err := data.DeserializeHTTPResponseData(httpResponsePacket.GetHttpResponseData(), s.commonConfig.Networking.Format, s.bucket)
					if err != nil {
						s.logger.Info("Error deserializing HTTP response data: " + err.Error())
						return
					}

					if httpResponsePacket.Compress == "zstd" && s.decoder != nil {
						var err error
						httpResponseData.Body.Body, err = s.decoder.DecodeAll(httpResponseData.Body.Body, nil)
						if err != nil {
							s.logger.Info("Error decompressing message: " + err.Error())
							return
						}
					}

					if httpResponseData.Body.Type == "split" {
						s.logger.Debug("Received split message")
						s.mergeCh <- mergeChPayload{
							responsePacket:   httpResponsePacket,
							httpResponseData: httpResponseData,
						}
					} else {
						s.logger.Debug("Received non-split message")
						s.busCh <- busChPayload{
							responsePacket:   httpResponsePacket,
							httpResponseData: httpResponseData,
						}
					}
				}()
			case mergeChPayload := <-s.mergeCh:
				go func() {
					combined, completed, err := split.Merge(s.merger, mergeChPayload.httpResponseData.Body.Body, s.commonConfig.Networking.Format)
					if err != nil {
						s.logger.Info("Error merging message: " + err.Error())
						return
					}
					if completed {
						mergeChPayload.httpResponseData.Body.Body = combined
						s.busCh <- busChPayload(mergeChPayload)
						s.merger.DeleteChunk(mergeChPayload.responsePacket.RequestId)
					}
				}()
			case busChPayload := <-s.busCh:
				go func() {
					s.logger.Debug("Emitting message to bus")

					err := s.bus.Emit(context.Background(), busChPayload.responsePacket.RequestId, busChPayload.httpResponseData)
					if err != nil {
						if strings.Contains(err.Error(), fmt.Sprintf("bus: topic(%s) not found", busChPayload.responsePacket.RequestId)) {
							return
						}
						s.logger.Error("Error emitting message to bus: " + err.Error())
						return
					}
				}()
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	c.Logger.Info("Gracefully shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.client.Disconnect(context.Background())
	if err != nil {
		c.Logger.Fatal("Error disconnecting from MQTT broker: " + err.Error())
	}
	close(s.mergeCh)

	if err := srv.Shutdown(ctx); err != nil {
		c.Logger.Fatal(fmt.Sprintf("shutting down error: %v", err))
	}
}
