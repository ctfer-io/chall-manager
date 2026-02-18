package global

import (
	"context"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/multierr"
)

const (
	// ServiceName is trace service name
	serviceName = "chall-manager"

	// DefaultSamplingRatio default sample ratio
	defaultSamplingRatio = 1
)

var (
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *log.LoggerProvider

	Tracer trace.Tracer = tracenoop.NewTracerProvider().Tracer(serviceName)
	Meter  metric.Meter = metricnoop.NewMeterProvider().Meter(serviceName)
)

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

func setupMeterProvider(ctx context.Context, r *resource.Resource) error {
	metricExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return err
	}

	meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(r),
	)
	Meter = meterProvider.Meter(serviceName)
	return nil
}

func setupLoggerProvider(ctx context.Context, r *resource.Resource) error {
	exp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return err
	}

	loggerProvider = log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(exp)),
		log.WithResource(r),
	)
	return nil
}

// SetupOTelSDK configures the OpenTelemetry signals exporters.
func SetupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
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

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned
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

	// Set up meter provider
	if nerr := setupMeterProvider(ctx, r); nerr != nil {
		handleErr(nerr)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

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
