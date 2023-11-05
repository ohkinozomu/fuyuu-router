package hub

import (
	"encoding/json"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"go.uber.org/zap"
)

type Router struct {
	db     *badger.DB
	logger *zap.Logger
}

var _ paho.Router = (*Router)(nil)

func NewRouter(db *badger.DB, logger *zap.Logger) *Router {
	return &Router{
		db:     db,
		logger: logger,
	}
}

func (r *Router) Route(p *packets.Publish) {
	responsePacket := data.HTTPResponsePacket{}
	if err := json.Unmarshal(p.Payload, &responsePacket); err != nil {
		r.logger.Info("Error unmarshalling message: " + err.Error())
		return
	}

	httpResponseData, err := json.Marshal(responsePacket.HTTPResponseData)
	if err != nil {
		r.logger.Info("Error marshalling HTTP response data: " + err.Error())
		return
	}

	err = r.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(responsePacket.RequestID), []byte(httpResponseData)).WithTTL(time.Minute * 5)
		err = txn.SetEntry(e)
		return err
	})
	if err != nil {
		r.logger.Info("Error setting key in database: " + err.Error())
		return
	}
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(paho.Logger) {}
