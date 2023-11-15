package common

import (
	"fmt"

	"go.uber.org/zap"
)

type zapToBadgerAdapter struct {
	logger *zap.Logger
}

func NewZapToBadgerAdapter(logger *zap.Logger) *zapToBadgerAdapter {
	return &zapToBadgerAdapter{logger: logger}
}

func (adapter *zapToBadgerAdapter) Errorf(format string, args ...any) {
	adapter.logger.Sugar().Errorf(format, args...)
}

func (adapter *zapToBadgerAdapter) Warningf(format string, args ...any) {
	adapter.logger.Sugar().Warnf(format, args...)
}

func (adapter *zapToBadgerAdapter) Infof(format string, args ...any) {
	adapter.logger.Sugar().Infof(format, args...)
}

func (adapter *zapToBadgerAdapter) Debugf(format string, args ...any) {
	adapter.logger.Sugar().Debugf(format, args...)
}

type zapToGoKitAdapter struct {
	logger *zap.Logger
}

func NewZapToGoKitAdapter(logger *zap.Logger) *zapToGoKitAdapter {
	return &zapToGoKitAdapter{logger: logger}
}

func (adapter *zapToGoKitAdapter) Log(keyvals ...any) error {
	if len(keyvals)%2 != 0 {
		return fmt.Errorf("keyvals must come in pairs")
	}

	fields := make([]zap.Field, len(keyvals)/2)
	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			return fmt.Errorf("key must be a string")
		}
		fields[i/2] = zap.Any(key, keyvals[i+1])
	}

	adapter.logger.With(fields...).Info("")
	return nil
}
