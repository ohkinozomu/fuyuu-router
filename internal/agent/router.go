package agent

import (
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"go.uber.org/zap"
)

type Router struct {
	payloadCh chan []byte
	logger    *zap.Logger
}

var _ paho.Router = (*Router)(nil)

func NewRouter(payloadCh chan []byte, c AgentConfig, logger *zap.Logger) *Router {
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
