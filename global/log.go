package global

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

type challenge string
type source string

var (
	challKey  = challenge("challenge")
	sourceKey = source("source")
)

type Logger struct {
	Sub *zap.Logger
}

func (log *Logger) Info(ctx context.Context, msg string, fields ...zap.Field) {
	log.Sub.Info(msg, decaps(ctx, fields...)...)
}

func (log *Logger) Error(ctx context.Context, msg string, fields ...zap.Field) {
	log.Sub.Error(msg, decaps(ctx, fields...)...)
}

func (log *Logger) Debug(ctx context.Context, msg string, fields ...zap.Field) {
	log.Sub.Debug(msg, decaps(ctx, fields...)...)
}

func (log *Logger) Warn(ctx context.Context, msg string, fields ...zap.Field) {
	log.Sub.Warn(msg, decaps(ctx, fields...)...)
}

func decaps(ctx context.Context, fields ...zap.Field) []zap.Field {
	if challId := ctx.Value(challKey); challId != nil {
		fields = append(fields, zap.String("challenge_id", challId.(string)))
	}
	if sourceId := ctx.Value(sourceKey); sourceId != nil {
		fields = append(fields, zap.String("source_id", sourceId.(string)))
	}
	return fields
}

func WithChallengeId(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, challKey, id)
}

func WithSourceId(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sourceKey, id)
}

var (
	logger  *Logger
	logOnce sync.Once
)

func Log() *Logger {
	logOnce.Do(func() {
		sub, _ := zap.NewProduction()
		logger = &Logger{
			Sub: sub,
		}
	})
	return logger
}
