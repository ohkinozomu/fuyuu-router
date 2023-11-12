package hub

import (
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"go.uber.org/zap"
)

type Router struct {
	db           *badger.DB
	logger       *zap.Logger
	commonConfig common.CommonConfigV2
}

var _ paho.Router = (*Router)(nil)

func NewRouter(db *badger.DB, logger *zap.Logger, commonConfig common.CommonConfigV2) *Router {
	return &Router{
		db:           db,
		logger:       logger,
		commonConfig: commonConfig,
	}
}

func (r *Router) Route(p *packets.Publish) {
	httpResponsePacket, err := data.DeserializeResponsePacket(p.Payload, r.commonConfig.Networking.Format)
	if err != nil {
		r.logger.Info("Error deserializing response packet: " + err.Error())
		return
	}

	err = r.db.Update(func(txn *badger.Txn) error {
		e1 := badger.NewEntry([]byte(httpResponsePacket.RequestId), httpResponsePacket.GetHttpResponseData()).WithTTL(time.Minute * 5)
		err = txn.SetEntry(e1)
		if err != nil {
			return err
		}
		e2 := badger.NewEntry([]byte("compress/"+httpResponsePacket.RequestId), []byte(httpResponsePacket.Compress)).WithTTL(time.Minute * 5)
		err = txn.SetEntry(e2)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		r.logger.Info("Error setting key in database: " + err.Error())
		return
	}
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(paho.Logger) {}
