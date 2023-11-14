package agent

import (
	"context"
	"time"

	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"go.uber.org/zap"
)

type Router struct {
	payloadCh chan []byte
	logger    *zap.Logger
}

var _ paho.Router = (*Router)(nil)

func NewRouter(payloadCh chan []byte, c AgentConfig, logger *zap.Logger) *Router {
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

	return &Router{
		payloadCh: payloadCh,
		logger:    logger,
	}
}

func (r *Router) Route(p *packets.Publish) {
	r.logger.Debug("Received message")
	r.payloadCh <- p.Payload
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(paho.Logger) {}
