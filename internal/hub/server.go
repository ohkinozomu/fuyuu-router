package hub

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
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
	"github.com/mustafaturan/monoton/v2"
	"github.com/mustafaturan/monoton/v2/sequencer"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
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

type updateDBChPayload struct {
	responsePacket   *data.HTTPResponsePacket
	httpResponseData *data.HTTPResponseData
}

type server struct {
	client       *autopaho.ConnectionManager
	bus          *bus.Bus
	logger       *zap.Logger
	encoder      *zstd.Encoder
	decoder      *zstd.Decoder
	commonConfig common.CommonConfigV2
	bucket       objstore.Bucket
	merger       *data.Merger
	dataCh       chan []byte
	payloadCh    chan []byte
	mergeCh      chan mergeChPayload
	updateDBCh   chan updateDBChPayload
	errCh        chan error
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

func newBus() *bus.Bus {
	node := uint64(1)
	initialTime := uint64(time.Now().Unix())
	m, err := monoton.New(sequencer.NewMillisecond(), node, initialTime)
	if err != nil {
		panic(err)
	}

	var idGenerator bus.Next = m.Next

	b, err := bus.NewBus(idGenerator)
	if err != nil {
		panic(err)
	}

	return b
}

func newServer(c HubConfig) server {
	var encoder *zstd.Encoder
	var decoder *zstd.Decoder
	var err error
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

	dataCh := make(chan []byte, 1000)
	payloadCh := make(chan []byte, 1000)
	mergeCh := make(chan mergeChPayload, 1000)
	updateDBCh := make(chan updateDBChPayload, 1000)
	errCh := make(chan error, 1000)
	client := newClient(c, payloadCh)

	if c.CommonConfigV2.Telemetry.Enabled {
		exporter, err := prometheus.New()
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		meterName := "fuyuu-router-hub"
		provider := metric.NewMeterProvider(metric.WithReader(exporter))
		meter := provider.Meter(meterName)

		dataChSize, err := meter.Float64ObservableGauge("buffered_data_channel_size", api.WithDescription("The size of the buffered dataCh"))
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
			n := float64(len(dataCh))
			o.ObserveFloat64(dataChSize, n)
			return nil
		}, dataChSize)
		if err != nil {
			c.Logger.Fatal(err.Error())
		}

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

		updateDBChSize, err := meter.Float64ObservableGauge("buffered_update_db_channel_size", api.WithDescription("The size of the buffered updateDBCh"))
		if err != nil {
			c.Logger.Fatal(err.Error())
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o api.Observer) error {
			n := float64(len(updateDBCh))
			o.ObserveFloat64(updateDBChSize, n)
			return nil
		}, updateDBChSize)
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
		client:       client,
		bus:          newBus(),
		logger:       c.Logger,
		encoder:      encoder,
		decoder:      decoder,
		commonConfig: c.CommonConfigV2,
		bucket:       bucket,
		merger:       data.NewMerger(c.Logger),
		dataCh:       dataCh,
		payloadCh:    payloadCh,
		mergeCh:      mergeCh,
		updateDBCh:   updateDBCh,
		errCh:        errCh,
	}
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling request...")

	fuyuuRouterIDs := r.Header.Get("FuyuuRouter-IDs")
	if fuyuuRouterIDs == "" {
		w.Write([]byte("FuyuuRouter-IDs header is required"))
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
		objectName = agentID + "/" + uuid + "/request"
		s.logger.Debug("Uploading object to object storage...")
		err := s.bucket.Upload(context.Background(), objectName, r.Body)
		if err != nil {
			s.logger.Error("Error uploading object to object storage", zap.Error(err))
			return
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
		s.errCh <- err
		return
	}
	defer s.client.Unsubscribe(ctx, &paho.Unsubscribe{
		Topics: []string{topic},
	})

	s.bus.RegisterTopics(uuid)
	handler := bus.Handler{
		Handle: func(ctx context.Context, e bus.Event) {
			s.logger.Debug("Received message from bus")
			s.dataCh <- e.Data.([]byte)
		},
		Matcher: uuid,
	}
	s.bus.RegisterHandler(uuid, handler)

	bodyBytes := make([]byte, r.ContentLength)
	r.Body.Read(bodyBytes)

	if s.encoder != nil {
		bodyBytes = s.encoder.EncodeAll(bodyBytes, nil)
	}

	dataHeaders := data.HTTPHeaderToProtoHeaders(r.Header)

	var body data.HTTPBody

	s.logger.Debug("content length: " + fmt.Sprintf("%d", r.ContentLength))
	s.logger.Debug("split threshold: " + fmt.Sprintf("%d", s.commonConfig.Split.ChunkBytes))
	if s.commonConfig.Networking.LargeDataPolicy == "split" && r.ContentLength > int64(s.commonConfig.Split.ChunkBytes) {
		s.logger.Debug("Splitting request body...")
		var chunks [][]byte
		for i := 0; i < len(bodyBytes); i += s.commonConfig.Split.ChunkBytes {
			end := i + s.commonConfig.Split.ChunkBytes
			if end > len(bodyBytes) {
				end = len(bodyBytes)
			}
			chunks = append(chunks, bodyBytes[i:end])
		}

		for sequence, c := range chunks {
			httpBodyChunk := data.HTTPBodyChunk{
				RequestId: uuid,
				Total:     int32(len(chunks)),
				Sequence:  int32(sequence + 1),
				Data:      c,
			}
			b, err := data.SerializeHTTPBodyChunk(&httpBodyChunk, s.commonConfig.Networking.Format)
			if err != nil {
				s.logger.Error("Error serializing body chunk", zap.Error(err))
				return
			}

			body = data.HTTPBody{
				Body: b,
				Type: "split",
			}

			requestData := data.HTTPRequestData{
				Method:  r.Method,
				Path:    r.URL.Path,
				Headers: &dataHeaders,
				Body:    &body,
			}

			b, err = data.SerializeHTTPRequestData(&requestData, s.commonConfig.Networking.Format)
			if err != nil {
				s.logger.Error("Error serializing request data", zap.Error(err))
				return
			}

			requestPacket := data.HTTPRequestPacket{
				RequestId:       uuid,
				HttpRequestData: b,
				Compress:        s.commonConfig.Networking.Compress,
			}

			requestPayload, err := data.SerializeRequestPacket(&requestPacket, s.commonConfig.Networking.Format)
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
			s.logger.Debug("Published request")
		}
	} else {
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
		requestData := data.HTTPRequestData{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: &dataHeaders,
			Body:    &body,
		}

		b, err := data.SerializeHTTPRequestData(&requestData, s.commonConfig.Networking.Format)
		if err != nil {
			s.logger.Error("Error serializing request data", zap.Error(err))
			return
		}

		requestPacket := data.HTTPRequestPacket{
			RequestId:       uuid,
			HttpRequestData: b,
			Compress:        s.commonConfig.Networking.Compress,
		}

		requestPayload, err := data.SerializeRequestPacket(&requestPacket, s.commonConfig.Networking.Format)
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
	case value := <-s.dataCh:
		s.logger.Debug("Writing response...")
		httpResponseData, err := data.DeserializeHTTPResponseData(value, s.commonConfig.Networking.Format, s.bucket)
		if err != nil {
			s.logger.Error("Error deserializing response data", zap.Error(err))
			return
		}
		if value != nil {
			w.WriteHeader(int(httpResponseData.StatusCode))
			w.Write([]byte(httpResponseData.Body.Body))
		}
	case <-ctx.Done():
		s.logger.Debug("Request timed out...")
		if ctx.Err() == context.DeadlineExceeded {
			w.WriteHeader(http.StatusRequestTimeout)
			w.Write([]byte("The request timed out."))
		}
		return
	}
	s.bus.DeregisterHandler(uuid)
	s.bus.DeregisterTopics(uuid)
}

func Start(c HubConfig) {
	c.Logger.Debug("config: " + fmt.Sprintf("%+v", c))
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
					httpResponsePacket, err := data.DeserializeResponsePacket(payload, s.commonConfig.Networking.Format)
					if err != nil {
						s.logger.Info("Error deserializing response packet: " + err.Error())
						return
					}

					httpResponseData, err := data.DeserializeHTTPResponseData(httpResponsePacket.GetHttpResponseData(), s.commonConfig.Networking.Format, nil)
					if err != nil {
						s.logger.Info("Error deserializing HTTP response data: " + err.Error())
						return
					}
					if httpResponseData.Body.Type == "split" {
						s.logger.Debug("Received split message")
						s.mergeCh <- mergeChPayload{
							responsePacket:   httpResponsePacket,
							httpResponseData: httpResponseData,
						}
					} else {
						s.logger.Debug("Received non-split message")
						s.updateDBCh <- updateDBChPayload{
							responsePacket:   httpResponsePacket,
							httpResponseData: httpResponseData,
						}
					}
				}()
			case mergeChPayload := <-s.mergeCh:
				go func() {
					chunk, err := data.DeserializeHTTPBodyChunk(mergeChPayload.httpResponseData.Body.Body, s.commonConfig.Networking.Format)
					if err != nil {
						s.logger.Error("Error deserializing HTTP body chunk", zap.Error(err))
						return
					}
					s.merger.AddChunk(chunk)

					if s.merger.IsComplete(chunk) {
						combined := s.merger.GetCombinedData(chunk)
						mergeChPayload.httpResponseData.Body.Body = combined
						s.updateDBCh <- updateDBChPayload(mergeChPayload)
					}
				}()
			case updateDBChPayload := <-s.updateDBCh:
				go func() {
					s.logger.Debug("Writing response to database...")

					if updateDBChPayload.responsePacket.Compress == "zstd" && s.decoder != nil {
						var err error
						updateDBChPayload.httpResponseData.Body.Body, err = s.decoder.DecodeAll(updateDBChPayload.httpResponseData.Body.Body, nil)
						if err != nil {
							s.logger.Info("Error decompressing message: " + err.Error())
							return
						}
					}

					b, err := data.SerializeHTTPResponseData(updateDBChPayload.httpResponseData, s.commonConfig.Networking.Format)
					if err != nil {
						s.logger.Info("Error serializing HTTP response data: " + err.Error())
						return
					}

					err = s.bus.Emit(context.Background(), updateDBChPayload.responsePacket.RequestId, b)
					if err != nil {
						s.logger.Info("Error setting key in database: " + err.Error())
						return
					}
				}()
			case err := <-s.errCh:
				s.logger.Info("Error: " + err.Error())
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
	close(s.dataCh)
	close(s.mergeCh)

	if err := srv.Shutdown(ctx); err != nil {
		c.Logger.Fatal(fmt.Sprintf("shutting down error: %v", err))
	}
}
