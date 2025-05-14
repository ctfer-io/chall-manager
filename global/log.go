package global

import (
	"context"
	"os"
	"sync"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	challengeKey struct{}
	sourceKey    struct{}
	identityKey  struct{}
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
	if challID := ctx.Value(challengeKey{}); challID != nil && challID.(string) != "" {
		fields = append(fields, zap.String("challenge_id", challID.(string)))
	}
	if identity := ctx.Value(identityKey{}); identity != nil && identity.(string) != "" {
		fields = append(fields, zap.String("identity", identity.(string)))
	}
	if sourceID := ctx.Value(sourceKey{}); sourceID != nil && sourceID.(string) != "" {
		fields = append(fields, zap.String("source_id", sourceID.(string)))
	}
	return fields
}

var (
	logger  *Logger
	logOnce sync.Once
)

func Log() *Logger {
	logOnce.Do(func() {
		sub, _ := zap.NewProduction()
		if Conf.Otel.Tracing {
			core := zapcore.NewTee(
				zapcore.NewCore(
					zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
					zapcore.AddSync(os.Stdout),
					zapcore.InfoLevel,
				),
				otelzap.NewCore(
					Conf.Otel.ServiceName,
					otelzap.WithLoggerProvider(loggerProvider),
				),
			)
			sub = zap.New(core)
		}

		logger = &Logger{
			Sub: sub,
		}
	})
	return logger
}
