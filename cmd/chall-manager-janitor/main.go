package main

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"

	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
	builtBy = ""

	tracing bool
)

func main() {
	app := &cli.App{
		Name:  "Chall-Manager-Janitor",
		Usage: "Chall-Manager-Janitor is an utility that handles challenges instances death.",
		Flags: []cli.Flag{
			cli.VersionFlag,
			cli.HelpFlag,
			&cli.StringFlag{
				Name:    "url",
				EnvVars: []string{"URL"},
				Usage:   "The chall-manager URL to reach out.",
			},
			&cli.BoolFlag{
				Name:        "tracing",
				EnvVars:     []string{"TRACING"},
				Usage:       "If set, turns on tracing through OpenTelemetry (see https://opentelemetry.io) for more info.",
				Destination: &tracing,
			},
		},
		Action: run,
		Authors: []*cli.Author{
			{
				Name:  "Lucas Tesson - PandatiX",
				Email: "lucastesson@protonmail.com",
			},
		},
		Version: version,
		Metadata: map[string]any{
			"version": version,
			"commit":  commit,
			"date":    date,
			"builtBy": builtBy,
		},
	}

	if err := app.Run(os.Args); err != nil {
		Log().Error(context.Background(), "fatal error",
			zap.Error(err),
		)
		os.Exit(1)
	}
}

func run(ctx *cli.Context) error {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	if tracing {
		opts = append(opts, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))

		// Set up OpenTelemetry.
		otelShutdown, err := setupOtelSDK(ctx.Context, "chall-manager-janitor")
		if err != nil {
			return err
		}
		// Handle shutdown properly so nothing leaks.
		defer func() {
			err = multierr.Append(err, otelShutdown(ctx.Context))
		}()
	}

	logger := Log()

	cli, _ := grpc.NewClient(ctx.String("url"), opts...)
	store := challenge.NewChallengeStoreClient(cli)
	manager := instance.NewInstanceManagerClient(cli)
	defer func(cli *grpc.ClientConn) {
		if err := cli.Close(); err != nil {
			logger.Error(ctx.Context, "closing gRPC connection", zap.Error(err))
		}
	}(cli)

	span := trace.SpanFromContext(ctx.Context)
	span.AddEvent("querying challenges")
	challs, err := store.QueryChallenge(ctx.Context, nil)
	if err != nil {
		return err
	}
	for {
		chall, err := challs.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		ctx := WithChallengeId(ctx.Context, chall.Id)

		// Don't janitor if the challenge has no dates configured
		if chall.Timeout == nil && chall.Until == nil {
			logger.Info(ctx, "skipping challenge with no dates configured")
			continue
		}

		// Janitor outdated instances
		wg := &sync.WaitGroup{}
		for _, ist := range chall.Instances {
			ctx := WithSourceId(ctx, ist.SourceId)

			if time.Now().After(ist.Until.AsTime()) {
				logger.Info(ctx, "janitoring instance")
				wg.Add(1)

				go func(ist *instance.Instance) {
					defer wg.Done()

					if _, err := manager.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
						ChallengeId: ist.ChallengeId,
						SourceId:    ist.SourceId,
					}); err != nil {
						logger.Error(ctx, "deleting challenge instance",
							zap.Error(err),
						)
					}
				}(ist)
			}
		}
		wg.Wait()
	}
	return nil
}

// region logger

type challengeKey struct{}
type sourceKey struct{}

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
	if challId := ctx.Value(challengeKey{}); challId != nil {
		fields = append(fields, zap.String("challenge_id", challId.(string)))
	}
	if sourceId := ctx.Value(sourceKey{}); sourceId != nil {
		fields = append(fields, zap.String("source_id", sourceId.(string)))
	}
	return fields
}

func WithChallengeId(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, challengeKey{}, id)
}

func WithSourceId(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sourceKey{}, id)
}

var (
	logger  *Logger
	logOnce sync.Once
)

func Log() *Logger {
	logOnce.Do(func() {
		sub, _ := zap.NewProduction()
		if tracing {
			core := zapcore.NewTee(
				zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(os.Stdout), zapcore.InfoLevel),
				otelzap.NewCore("chall-manager-janitor", otelzap.WithLoggerProvider(loggerProvider)),
			)
			sub = zap.New(core)
		}

		logger = &Logger{
			Sub: sub,
		}
	})
	return logger
}

// region OTEL

var (
	tracerProvider *sdktrace.TracerProvider
	loggerProvider *log.LoggerProvider

	Tracer trace.Tracer = tracenoop.NewTracerProvider().Tracer("")
)

// setupOtelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOtelSDK(ctx context.Context, name string) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = multierr.Append(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = multierr.Append(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Ensure default SDK resources and the required service name are set.
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(name),
		),
	)
	if err != nil {
		return nil, err
	}

	// Set up trace provider.
	if nerr := setupTraceProvider(r, ctx, name); nerr != nil {
		handleErr(nerr)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up logger provider.
	if nerr := setupLoggerProvider(r, ctx); nerr != nil {
		handleErr(nerr)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	otelglobal.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func setupTraceProvider(r *resource.Resource, ctx context.Context, name string) error {
	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return err
	}

	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(r),
	)
	Tracer = tracerProvider.Tracer(name)
	return nil
}

func setupLoggerProvider(r *resource.Resource, ctx context.Context) error {
	logExporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return err
	}

	loggerProvider = log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
		log.WithResource(r),
	)
	return nil
}
