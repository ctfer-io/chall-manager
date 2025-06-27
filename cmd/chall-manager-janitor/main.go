package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	cmotel "github.com/ctfer-io/chall-manager/pkg/otel"

	"github.com/sony/gobreaker/v2"
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

	tracing     bool
	serviceName string

	cb *gobreaker.CircuitBreaker[grpc.ServerStreamingClient[challenge.Challenge]]
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
			&cli.StringFlag{
				Name:     "log-level",
				EnvVars:  []string{"LOG_LEVEL"},
				Category: "global",
				Value:    "info",
				Action: func(_ *cli.Context, lvl string) error {
					_, err := zapcore.ParseLevel(lvl)
					return err
				},
				Destination: &LogLevel,
				Usage:       "Use to specify the level of logging.",
			},
			&cli.BoolFlag{
				Name:        "tracing",
				EnvVars:     []string{"TRACING"},
				Category:    "otel",
				Usage:       "If set, turns on tracing through OpenTelemetry (see https://opentelemetry.io) for more info.",
				Destination: &tracing,
			},
			&cli.StringFlag{
				Name:        "service-name",
				EnvVars:     []string{"OTEL_SERVICE_NAME"},
				Category:    "otel",
				Value:       "chall-manager-janitor",
				Destination: &serviceName,
				Usage:       "Override the service name. Useful when deploying multiple instances to filter signals.",
			},
			&cli.DurationFlag{
				Name:    "ticker",
				EnvVars: []string{"TICKER"},
				Usage: `If set, define the tick between 2 run of the janitor. ` +
					`This mode is not recommended and should be preferred to a cron or an equivalent.`,
				// Not recommended because the janitor was not made to be a long-running software.
				// It was not optimised in this way, despite it should work fine.
			},
			&cli.IntFlag{
				Name:     "max-requests",
				Category: "resiliency",
				Usage: "The maximum number of requests allowed to pass through when the " +
					"circuit breaker is half-open.",
				Action: func(_ *cli.Context, i int) error {
					if i < 0 {
						return errors.New("max-requests cannot be negative")
					}
					if i > math.MaxUint32 {
						return fmt.Errorf("max-requests is too high, maximum allowed is %d", math.MaxUint32)
					}
					return nil
				},
			},
			&cli.DurationFlag{
				Name:     "interval",
				Category: "resiliency",
				Usage: "The cyclic period of the closed state for the circuit breaker to " +
					"clean the internal counts.",
			},
			&cli.DurationFlag{
				Name:     "timeout",
				Category: "resiliency",
				Usage: "The period of the open state after which the state of the circuit " +
					"breaker becomes half-open.",
				Value: must(time.ParseDuration("10s")),
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

func run(c *cli.Context) error {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	if tracing {
		// Set up OpenTelemetry.
		otelShutdown, err := setupOtelSDK(c.Context)
		if err != nil {
			return err
		}
		// Handle shutdown properly so nothing leaks.
		defer func() {
			err = multierr.Append(err, otelShutdown(c.Context))
		}()

		opts = append(opts,
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			grpc.WithUnaryInterceptor(cmotel.UnaryClientInterceptorWithCaller(Tracer)),
			grpc.WithStreamInterceptor(cmotel.StreamClientInterceptorWithCaller(Tracer)),
		)
	}

	logger := Log()

	// Setup the circuit breaker to chall-manager
	cb = gobreaker.NewCircuitBreaker[grpc.ServerStreamingClient[challenge.Challenge]](gobreaker.Settings{
		Name:        "chall-manager",
		MaxRequests: uint32(c.Int("max-requests")), //nolint:gosec
		Interval:    c.Duration("interval"),
		Timeout:     c.Duration("timeout"),
	})

	// Create context that listens for the interrupt signal from the OS
	ctx, stop := signal.NotifyContext(c.Context, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cli, err := grpc.NewClient(c.String("url"), opts...)
	if err != nil {
		return err
	}
	defer func(cli *grpc.ClientConn) {
		if err := cli.Close(); err != nil {
			logger.Error(ctx, "closing gRPC connection", zap.Error(err))
		}
	}(cli)

	if c.IsSet("ticker") {
		if err := janitorWithTicker(ctx, cli, c.Duration("ticker")); err != nil {
			return err
		}
	} else {
		if err := janitor(ctx, cli); err != nil {
			return err
		}
		stop()
	}

	// Listen for the interrupt signal
	<-ctx.Done()

	// Restore default behavior on the interrupt signal
	stop()
	logger.Info(ctx, "shutting down gracefully")

	return nil
}

func janitorWithTicker(ctx context.Context, cli *grpc.ClientConn, d time.Duration) error {
	logger := Log()
	ticker := time.NewTicker(d)
	wg := sync.WaitGroup{}

	run := true
	for run {
		select {
		case <-ticker.C:
			wg.Add(1)
			go func(ctx context.Context, cli *grpc.ClientConn) {
				defer wg.Done()

				if err := janitor(ctx, cli); err != nil {
					logger.Error(ctx, "janitoring did not succeed", zap.Error(err))
				}
			}(ctx, cli)

		case <-ctx.Done():
			ticker.Stop()
			run = false
		}
	}
	wg.Wait()
	return nil
}

func janitor(ctx context.Context, cli *grpc.ClientConn) error {
	logger := Log()
	logger.Info(ctx, "starting janitoring")

	store := challenge.NewChallengeStoreClient(cli)
	manager := instance.NewInstanceManagerClient(cli)

	span := trace.SpanFromContext(ctx)
	span.AddEvent("querying challenges")

	challs, err := cb.Execute(func() (grpc.ServerStreamingClient[challenge.Challenge], error) {
		return store.QueryChallenge(ctx, nil)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			logger.Error(ctx, "downstream service seems not available",
				zap.String("service", "chall-manager"),
				zap.String("state", cb.State().String()),
			)
			return nil
		}
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
		ctx := WithChallengeID(ctx, chall.Id)

		// Don't janitor if the challenge has no dates configured
		if chall.Timeout == nil && chall.Until == nil {
			logger.Info(ctx, "skipping challenge with no dates configured")
			continue
		}

		// Janitor outdated instances
		wg := &sync.WaitGroup{}
		for _, ist := range chall.Instances {
			ctx := WithSourceID(ctx, ist.SourceId)

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

	logger.Info(ctx, "completed janitoring")

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
	if challID := ctx.Value(challengeKey{}); challID != nil {
		fields = append(fields, zap.String("challenge_id", challID.(string)))
	}
	if sourceID := ctx.Value(sourceKey{}); sourceID != nil {
		fields = append(fields, zap.String("source_id", sourceID.(string)))
	}
	return fields
}

func WithChallengeID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, challengeKey{}, id)
}

func WithSourceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sourceKey{}, id)
}

var (
	logger  *Logger
	logOnce sync.Once

	LogLevel string
)

func Log() *Logger {
	logOnce.Do(func() {
		sub, _ := zap.NewProduction()
		if tracing {
			lvl, _ := zapcore.ParseLevel(LogLevel)
			core := zapcore.NewTee(
				zapcore.NewCore(
					zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
					zapcore.AddSync(os.Stdout),
					lvl,
				),
				otelzap.NewCore(
					"chall-manager-janitor",
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

// region OTEL

var (
	tracerProvider *sdktrace.TracerProvider
	loggerProvider *log.LoggerProvider

	Tracer trace.Tracer = tracenoop.NewTracerProvider().Tracer("")
)

// setupOtelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOtelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
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
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, err
	}

	// Set up trace provider.
	if nerr := setupTraceProvider(ctx, r); nerr != nil {
		handleErr(nerr)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up logger provider.
	if nerr := setupLoggerProvider(ctx, r); nerr != nil {
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

func setupTraceProvider(ctx context.Context, r *resource.Resource) error {
	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return err
	}

	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(r),
	)
	Tracer = tracerProvider.Tracer(serviceName)
	return nil
}

func setupLoggerProvider(ctx context.Context, r *resource.Resource) error {
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

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
