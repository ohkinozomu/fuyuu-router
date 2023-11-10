package hub

import (
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/klauspost/compress/zstd"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"go.uber.org/zap"
)

type Router struct {
	db      *badger.DB
	logger  *zap.Logger
	format  string
	decoder *zstd.Decoder
}

var _ paho.Router = (*Router)(nil)

func NewRouter(db *badger.DB, logger *zap.Logger, format, compress string) *Router {
	var decoder *zstd.Decoder
	var err error
	if compress == "zstd" {
		logger.Debug("Initializing zstd decoder...")
		decoder, err = zstd.NewReader(nil)
		if err != nil {
			logger.Fatal(err.Error())
		}
	} else if compress != "none" {
		logger.Fatal("Unknown compress: " + compress)
	}

	return &Router{
		db:      db,
		logger:  logger,
		format:  format,
		decoder: decoder,
	}
}

func (r *Router) Route(p *packets.Publish) {
	httpResponsePacket, err := data.DeserializeResponsePacket(p.Payload, r.format, r.decoder)
	if err != nil {
		r.logger.Info("Error deserializing response packet: " + err.Error())
		return
	}

	httpResponseData, err := data.SerializeHTTPResponseData(httpResponsePacket.GetHttpResponseData(), r.format)
	if err != nil {
		r.logger.Info("Error serializing response packet: " + err.Error())
		return
	}

	err = r.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(httpResponsePacket.RequestId), httpResponseData).WithTTL(time.Minute * 5)
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
