package hub

import (
	badger "github.com/dgraph-io/badger/v4"
	"github.com/eclipse/paho.golang/packets"
	"github.com/eclipse/paho.golang/paho"
	"github.com/eclipse/paho.golang/paho/log"
	"github.com/klauspost/compress/zstd"
	"github.com/ohkinozomu/fuyuu-router/internal/common"
	"github.com/ohkinozomu/fuyuu-router/pkg/data"
	"go.uber.org/zap"
)

type Router struct {
	db           *badger.DB
	logger       *zap.Logger
	commonConfig common.CommonConfigV2
	encoder      *zstd.Encoder
	decoder      *zstd.Decoder
	mergeCh      chan mergeChPayload
	updateDBCh   chan updateDBChPayload
}

var _ paho.Router = (*Router)(nil)

func NewRouter(db *badger.DB, logger *zap.Logger, commonConfig common.CommonConfigV2, encoder *zstd.Encoder, decoder *zstd.Decoder, mergeCh chan mergeChPayload, updateDBCh chan updateDBChPayload) *Router {
	return &Router{
		db:           db,
		logger:       logger,
		commonConfig: commonConfig,
		encoder:      encoder,
		decoder:      decoder,
		mergeCh:      mergeCh,
		updateDBCh:   updateDBCh,
	}
}

func (r *Router) Route(p *packets.Publish) {
	r.logger.Debug("Received message")
	httpResponsePacket, err := data.DeserializeResponsePacket(p.Payload, r.commonConfig.Networking.Format)
	if err != nil {
		r.logger.Info("Error deserializing response packet: " + err.Error())
		return
	}

	httpResponseData, err := data.DeserializeHTTPResponseData(httpResponsePacket.GetHttpResponseData(), r.commonConfig.Networking.Format, nil)
	if err != nil {
		r.logger.Info("Error deserializing HTTP response data: " + err.Error())
		return
	}
	if httpResponseData.Body.Type == "split" {
		r.mergeCh <- mergeChPayload{
			responsePacket:   httpResponsePacket,
			httpResponseData: httpResponseData,
		}
	} else {
		r.logger.Debug("Received non-split message")
		r.updateDBCh <- updateDBChPayload{
			responsePacket:   httpResponsePacket,
			httpResponseData: httpResponseData,
		}
	}
}

func (r *Router) RegisterHandler(string, paho.MessageHandler) {}

func (r *Router) UnregisterHandler(string) {}

func (r *Router) SetDebugLogger(log.Logger) {}
