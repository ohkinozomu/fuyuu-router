package hub

import (
	"encoding/json"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type Router struct {
	db     *badger.DB
	logger *zap.Logger
	format string
}

var _ paho.Router = (*Router)(nil)

func NewRouter(db *badger.DB, logger *zap.Logger, format string) *Router {
	return &Router{
		db:     db,
		logger: logger,
		format: format,
	}
}

func (r *Router) Route(p *packets.Publish) {
	responsePacket := data.HTTPResponsePacket{}

	if r.format == "json" {
		if err := json.Unmarshal(p.Payload, &responsePacket); err != nil {
			r.logger.Error("Error unmarshalling message", zap.Error(err))
			return
		}
	} else if r.format == "protobuf" {
		if err := proto.Unmarshal(p.Payload, &responsePacket); err != nil {
			r.logger.Error("Error unmarshalling message", zap.Error(err))
			return
		}
	} else {
		r.logger.Error("Unknown format: " + r.format)
		return
	}

	var httpResponseData []byte
	var err error
	if r.format == "json" {
		httpResponseData, err = json.Marshal(responsePacket.HttpResponseData)
		if err != nil {
			r.logger.Fatal(err.Error())
		}
	} else if r.format == "protobuf" {
		httpResponseData, err = proto.Marshal(responsePacket.HttpResponseData)
		if err != nil {
			r.logger.Fatal(err.Error())
		}
	} else {
		r.logger.Fatal("Unknown format: " + r.format)
	}

	err = r.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(responsePacket.RequestId), httpResponseData).WithTTL(time.Minute * 5)
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
