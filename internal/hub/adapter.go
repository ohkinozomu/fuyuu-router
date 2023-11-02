package hub

import (
	"go.uber.org/zap"
)

type zapToBadgerAdapter struct {
	logger *zap.Logger
}

func newZapToBadgerAdapter(logger *zap.Logger) *zapToBadgerAdapter {
	return &zapToBadgerAdapter{logger: logger}
}

func (adapter *zapToBadgerAdapter) Errorf(format string, args ...interface{}) {
	adapter.logger.Sugar().Errorf(format, args...)
}

func (adapter *zapToBadgerAdapter) Warningf(format string, args ...interface{}) {
	adapter.logger.Sugar().Warnf(format, args...)
}

func (adapter *zapToBadgerAdapter) Infof(format string, args ...interface{}) {
	adapter.logger.Sugar().Infof(format, args...)
}

func (adapter *zapToBadgerAdapter) Debugf(format string, args ...interface{}) {
	adapter.logger.Sugar().Debugf(format, args...)
}
