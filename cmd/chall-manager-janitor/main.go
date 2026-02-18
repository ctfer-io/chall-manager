package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/mail"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	cmotel "github.com/ctfer-io/chall-manager/pkg/otel"

	"github.com/sony/gobreaker/v2"
	"github.com/urfave/cli/v3"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
	BuiltBy = ""

	cb *gobreaker.CircuitBreaker[grpc.ServerStreamingClient[challenge.Challenge]]
)

const (
	// ServiceName is trace service name
	serviceName = "chall-manager-janitor"

	// DefaultSamplingRatio default sample ratio
	defaultSamplingRatio = 1
)

func main() {
	cmd := &cli.Command{
		Name:  "Chall-Manager-Janitor",
		Usage: "Chall-Manager-Janitor is an utility that handles challenges instances death.",
		Flags: []cli.Flag{
			cli.VersionFlag,
			cli.HelpFlag,
			&cli.StringFlag{
				Name:    "url",
				Sources: cli.EnvVars("URL"),
				Usage:   "The chall-manager URL to reach out.",
			},
			&cli.StringFlag{
				Name:     "log-level",
				Sources:  cli.EnvVars("LOG_LEVEL"),
				Category: "global",
				Value:    "info",
				Action: func(_ context.Context, _ *cli.Command, lvl string) error {
					_, err := zapcore.ParseLevel(lvl)
					return err
				},
				Destination: &LogLevel,
				Usage:       "Use to specify the level of logging.",
			},
			&cli.DurationFlag{
				Name:    "ticker",
				Sources: cli.EnvVars("TICKER"),
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
				Action: func(_ context.Context, cmd *cli.Command, i int) error {
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
		Authors: []any{
			mail.Address{
				Name:    "Lucas Tesson - PandatiX",
				Address: "lucastesson@protonmail.com",
			},
		},
		Version: Version,
		Metadata: map[string]any{
			"version": Version,
			"commit":  Commit,
			"date":    Date,
			"builtBy": BuiltBy,
		},
	}

	ctx := context.Background()
	if err := cmd.Run(ctx, os.Args); err != nil {
		Log().Error(ctx, "fatal error",
			zap.Error(err),
		)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	// Set up OpenTelemetry
	otelShutdown, err := setupOtelSDK(ctx)
	if err != nil {
		return err
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		err = multierr.Append(err, otelShutdown(ctx))
	}()

	opts = append(opts,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithUnaryInterceptor(cmotel.UnaryClientInterceptorWithCaller(Tracer)),
		grpc.WithStreamInterceptor(cmotel.StreamClientInterceptorWithCaller(Tracer)),
	)

	logger := Log()

	// Setup the circuit breaker to chall-manager
	cb = gobreaker.NewCircuitBreaker[grpc.ServerStreamingClient[challenge.Challenge]](gobreaker.Settings{
		Name:        "chall-manager",
		MaxRequests: uint32(cmd.Int("max-requests")), //nolint:gosec
		Interval:    cmd.Duration("interval"),
		Timeout:     cmd.Duration("timeout"),
	})

	// Create context that listens for the interrupt signal from the OS
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cli, err := grpc.NewClient(cmd.String("url"), opts...)
	if err != nil {
		return err
	}
	defer func(cli *grpc.ClientConn) {
		if err := cli.Close(); err != nil {
			logger.Error(ctx, "closing gRPC connection", zap.Error(err))
		}
	}(cli)

	if cmd.IsSet("ticker") {
		if err := janitorWithTicker(ctx, cli, cmd.Duration("ticker")); err != nil {
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
			logger.Debug(ctx, "skipping challenge with no dates configured")
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
	// Business layer fields
	if challID := ctx.Value(challengeKey{}); challID != nil {
		fields = append(fields, zap.String("challenge_id", challID.(string)))
	}
	if sourceID := ctx.Value(sourceKey{}); sourceID != nil {
		fields = append(fields, zap.String("source_id", sourceID.(string)))
	}

	// Tracing fields
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		fields = append(fields, zap.String("trace_id", span.SpanContext().TraceID().String()))
	}
	if span.SpanContext().HasSpanID() {
		fields = append(fields, zap.String("span_id", span.SpanContext().SpanID().String()))
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

		logger = &Logger{
			Sub: zap.New(core),
		}
	})
	return logger
}

// region OTel

var (
	tracerProvider *sdktrace.TracerProvider
	loggerProvider *log.LoggerProvider

	Tracer trace.Tracer = tracenoop.NewTracerProvider().Tracer(serviceName)
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

	// Define this OTel resource
	r, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(Version),
		),
		resource.WithFromEnv(), // define all other according to OTel conventions, i.e., from environment variables
	)
	if err != nil {
		return nil, err
	}

	// Set up trace provider
	if nerr := setupTraceProvider(ctx, r); nerr != nil {
		handleErr(nerr)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up logger provider
	if nerr := setupLoggerProvider(ctx, r); nerr != nil {
		handleErr(nerr)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	otelglobal.SetLoggerProvider(loggerProvider)

	// Set up propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return
}

func setupTraceProvider(ctx context.Context, r *resource.Resource) error {
	exp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return err
	}

	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(defaultSamplingRatio)),
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	)
	Tracer = tracerProvider.Tracer(serviceName)
	return nil
}

func setupLoggerProvider(ctx context.Context, r *resource.Resource) error {
	logExporter, err := autoexport.NewLogExporter(ctx)
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
