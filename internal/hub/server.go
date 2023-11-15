package hub

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/pb"
	"github.com/eclipse/paho.golang/paho"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/klauspost/compress/zstd"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"github.com/ohkinozomu/fuyuu-router/pkg/topics"
	"github.com/thanos-io/objstore"
	objstoreclient "github.com/thanos-io/objstore/client"
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
	requestClient  *paho.Client
	responseClient *paho.Client
	db             *badger.DB
	logger         *zap.Logger
	encoder        *zstd.Encoder
	decoder        *zstd.Decoder
	commonConfig   common.CommonConfigV2
	bucket         objstore.Bucket
	merger         *data.Merger
	dataCh         chan []byte
	mergeCh        chan mergeChPayload
	updateDBCh     chan updateDBChPayload
}

func newServer(c HubConfig) server {
	c.Logger.Debug("Opening database...")
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(common.NewZapToBadgerAdapter(c.Logger)))
	if err != nil {
		c.Logger.Fatal("Error opening database: " + err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c.Logger.Debug("Connecting to MQTT broker...")
	responseConn, err := common.TCPConnect(ctx, c.CommonConfig)
	if err != nil {
		c.Logger.Fatal("Error: " + err.Error())
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

	dataCh := make(chan []byte)
	mergeCh := make(chan mergeChPayload)
	updateDBCh := make(chan updateDBChPayload)
	responseClientConfig := paho.ClientConfig{
		Conn:   responseConn,
		Router: NewRouter(db, c.Logger, c.CommonConfigV2, encoder, decoder, mergeCh, updateDBCh),
	}
	responseClient := paho.NewClient(responseClientConfig)

	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c.Logger.Debug("Connecting to MQTT broker...")
	requestConn, err := common.TCPConnect(ctx, c.CommonConfig)
	if err != nil {
		c.Logger.Fatal("Error: " + err.Error())
	}
	requestClientConfig := paho.ClientConfig{
		Conn: requestConn,
	}
	requestClient := paho.NewClient(requestClientConfig)

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
		requestClient:  requestClient,
		responseClient: responseClient,
		db:             db,
		logger:         c.Logger,
		encoder:        encoder,
		decoder:        decoder,
		commonConfig:   c.CommonConfigV2,
		bucket:         bucket,
		merger:         data.NewMerger(),
		dataCh:         dataCh,
		mergeCh:        mergeCh,
		updateDBCh:     updateDBCh,
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
	_, err := s.responseClient.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{
				Topic: topic,
				QoS:   0,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	defer s.responseClient.Unsubscribe(ctx, &paho.Unsubscribe{
		Topics: []string{topic},
	})

	match := pb.Match{Prefix: []byte(uuid), IgnoreBytes: ""}

	go func() {
		err := s.db.Subscribe(ctx, func(kvs *pb.KVList) error {
			s.logger.Debug("Received data from database...")
			s.dataCh <- kvs.Kv[0].GetValue()
			return nil
		}, []pb.Match{match})
		if err != nil {
			if err != context.Canceled {
				s.logger.Info("Error subscribing to database: " + err.Error())
			}
		}
	}()

	bodyBytes := make([]byte, r.ContentLength)
	r.Body.Read(bodyBytes)

	dataHeaders := data.HTTPHeaderToProtoHeaders(r.Header)

	var body data.HTTPBody

	if s.commonConfig.Networking.LargeDataPolicy == "split" && r.ContentLength > int64(s.commonConfig.Split.ChunkBytes) {
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

			b, err = data.SerializeHTTPRequestData(&requestData, s.commonConfig.Networking.Format, s.encoder)
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
			_, err = s.requestClient.Publish(context.Background(), &paho.Publish{
				Payload: requestPayload,
				Topic:   requestTopic,
			})
			if err != nil {
				s.logger.Error("Error publishing request", zap.Error(err))
				return
			}
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

		b, err := data.SerializeHTTPRequestData(&requestData, s.commonConfig.Networking.Format, s.encoder)
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
		_, err = s.requestClient.Publish(context.Background(), &paho.Publish{
			Payload: requestPayload,
			Topic:   requestTopic,
		})
		if err != nil {
			s.logger.Error("Error publishing request", zap.Error(err))
			return
		}
		s.logger.Debug("Subscribing to response topic...")
	}

	select {
	case value := <-s.dataCh:
		s.logger.Debug("Writing response...")
		var compress string
		err := s.db.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte("compress/" + uuid))
			if err != nil {
				return err
			}
			err = item.Value(func(val []byte) error {
				compress = string(val)
				return nil
			})
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			s.logger.Error("Error getting compress value from database", zap.Error(err))
			return
		}
		httpResponseData, err := data.DeserializeHTTPResponseData(value, compress, s.commonConfig.Networking.Format, s.decoder, s.bucket)
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
}

func Start(c HubConfig) {
	s := newServer(c)

	connect := common.MQTTConnect(c.CommonConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	s.logger.Debug("Connecting to MQTT broker...")
	_, err := s.requestClient.Connect(ctx, connect)
	if err != nil {
		c.Logger.Fatal("Error connecting to MQTT broker: " + err.Error())
	}

	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	s.logger.Debug("Connecting to MQTT broker...")
	_, err = s.responseClient.Connect(ctx, connect)
	if err != nil {
		c.Logger.Fatal("Error connecting to MQTT broker: " + err.Error())
	}

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
			case mergeChPayload := <-s.mergeCh:
				chunk, err := data.DeserializeHTTPBodyChunk(mergeChPayload.httpResponseData.Body.Body, s.commonConfig.Networking.Format)
				if err != nil {
					s.logger.Error("Error deserializing HTTP body chunk", zap.Error(err))
					return
				}
				s.merger.AddChunk(chunk)

				if s.merger.IsComplete(chunk) {
					combined := s.merger.GetCombinedData(chunk)
					mergeChPayload.httpResponseData.Body.Body = combined
					updateDBChPayload := updateDBChPayload{
						responsePacket:   mergeChPayload.responsePacket,
						httpResponseData: mergeChPayload.httpResponseData,
					}
					s.updateDBCh <- updateDBChPayload
				}
			case updateDBChPayload := <-s.updateDBCh:
				s.logger.Debug("Writing response to database...")
				err := s.db.Update(func(txn *badger.Txn) error {
					e1 := badger.NewEntry([]byte(updateDBChPayload.responsePacket.RequestId), updateDBChPayload.responsePacket.GetHttpResponseData()).WithTTL(time.Minute * 5)
					err := txn.SetEntry(e1)
					if err != nil {
						return err
					}
					e2 := badger.NewEntry([]byte("compress/"+updateDBChPayload.responsePacket.RequestId), []byte(updateDBChPayload.responsePacket.Compress)).WithTTL(time.Minute * 5)
					err = txn.SetEntry(e2)
					if err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					s.logger.Info("Error setting key in database: " + err.Error())
					return
				}
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	c.Logger.Info("Gracefully shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.requestClient.Disconnect(&paho.Disconnect{ReasonCode: 0})
	if err != nil {
		c.Logger.Fatal("Error disconnecting from MQTT broker: " + err.Error())
	}
	err = s.responseClient.Disconnect(&paho.Disconnect{ReasonCode: 0})
	if err != nil {
		c.Logger.Fatal("Error disconnecting from MQTT broker: " + err.Error())
	}
	err = s.db.Close()
	if err != nil {
		c.Logger.Fatal("Error closing database: " + err.Error())
	}
	close(s.dataCh)
	close(s.mergeCh)

	if err := srv.Shutdown(ctx); err != nil {
		c.Logger.Fatal(fmt.Sprintf("shutting down error: %v", err))
	}
}
