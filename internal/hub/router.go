package hub

import (
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/eclipse/paho.golang/paho/log"
)

type Router struct {
	payloadCh chan []byte
}

var _ paho.Router = (*Router)(nil)

func NewRouter(payloadCh chan []byte) *Router {
	return &Router{
		payloadCh: payloadCh,
	}
}

func (r *Router) Route(p *packets.Publish) {
	r.payloadCh <- p.Payload
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(log.Logger) {}
