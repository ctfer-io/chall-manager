package global

import (
	"sync"

	"go.uber.org/zap"
)

var (
	logger  *zap.Logger
	logOnce sync.Once
)

func Log() *zap.Logger {
	logOnce.Do(func() {
		logger, _ = zap.NewProduction()
	})
	return logger
}
