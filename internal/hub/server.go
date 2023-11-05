package hub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/internal/data"
	"go.uber.org/zap"
)

type server struct {
	requestClient  *paho.Client
	responseClient *paho.Client
	db             *badger.DB
	logger         *zap.Logger
}

func newServer(c HubConfig) server {
	c.Logger.Debug("Opening database...")
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(newZapToBadgerAdapter(c.Logger)))
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
	responseClientConfig := paho.ClientConfig{
		Conn:   responseConn,
		Router: NewRouter(db, c.Logger),
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

	return server{
		requestClient:  requestClient,
		responseClient: responseClient,
		db:             db,
		logger:         c.Logger,
	}
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling request...")
	fuyuuRouterIDs := r.Header.Get("FuyuuRouter-IDs")
	agentIDs := strings.Split(fuyuuRouterIDs, ",")
	if len(agentIDs) == 0 {
		w.Write([]byte("FuyuuRouter-IDs header is required"))
		return
	}
	// Load Balancing
	// Now, select randomly
	agentID := agentIDs[rand.Intn(len(agentIDs))]

	timeoutDuration := 30 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeoutDuration)
	defer cancel()

	go func() {
		<-r.Context().Done()
		cancel()
	}()

	uuid := uuid.New().String()
	topic := common.ResponseTopic(agentID, uuid)

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

	dataCh := make(chan []byte)

	go func() {
		err := s.db.Subscribe(ctx, func(kvs *pb.KVList) error {
			s.logger.Debug("Received data from database...")
			dataCh <- kvs.Kv[0].GetValue()
			return nil
		}, []pb.Match{match})
		if err != nil {
			if err != context.Canceled {
				s.logger.Info("Error subscribing to database: " + err.Error())
			}
		}
		close(dataCh)
	}()

	bodyBytes := make([]byte, r.ContentLength)
	r.Body.Read(bodyBytes)

	requestData := data.HTTPRequestData{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: r.Header,
		Body:    base64.StdEncoding.EncodeToString(bodyBytes),
	}
	requestPacket := data.HTTPRequestPacket{
		RequestID:       uuid,
		HTTPRequestData: requestData,
	}

	jsonData, err := json.Marshal(requestPacket)
	if err != nil {
		panic(err)
	}
	requestTopic := common.RequestTopic(agentID)
	s.logger.Debug("Publishing request...")
	s.requestClient.Publish(context.Background(), &paho.Publish{
		Payload: jsonData,
		Topic:   requestTopic,
	})

	select {
	case value := <-dataCh:
		s.logger.Debug("Writing response...")
		var httpResponseData data.HTTPResponseData
		err := json.Unmarshal(value, &httpResponseData)
		if err != nil {
			s.logger.Info("Error unmarshalling response data: " + err.Error())
			return
		}
		if value != nil {
			w.WriteHeader(httpResponseData.StatusCode)
			w.Write([]byte(httpResponseData.Body))
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

	if c.Protocol == "http1" {
		s.startHTTP1(c)
	} else {
		c.Logger.Fatal("Unknown protocol: " + c.Protocol)
	}
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
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	c.Logger.Info("Gracefully shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.requestClient.Disconnect(&paho.Disconnect{ReasonCode: 0})
	s.responseClient.Disconnect(&paho.Disconnect{ReasonCode: 0})
	s.db.Close()

	if err := srv.Shutdown(ctx); err != nil {
		c.Logger.Fatal(fmt.Sprintf("shutting down error: %v", err))
	}
}
