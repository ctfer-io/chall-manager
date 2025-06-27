package otel

import (
	"context"
	"runtime"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

func UnaryClientInterceptorWithCaller(tracer trace.Tracer) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Determine caller name
		pc, _, _, ok := runtime.Caller(2)
		caller := "unknown"
		if ok {
			if fn := runtime.FuncForPC(pc); fn != nil {
				caller = fn.Name()
			}
		}

		// Start span for the unary
		ctx, span := tracer.Start(ctx, method)
		defer span.End()
		span.SetAttributes(attribute.String("caller.function", caller))

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func StreamClientInterceptorWithCaller(tracer trace.Tracer) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Determine caller name
		pc, _, _, ok := runtime.Caller(2)
		caller := "unknown"
		if ok {
			if fn := runtime.FuncForPC(pc); fn != nil {
				caller = fn.Name()
			}
		}

		// Start span for the stream
		ctx, span := tracer.Start(ctx, method)
		span.SetAttributes(attribute.String("caller.function", caller))

		// Call the actual streamer to get the client stream
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.RecordError(err)
			span.End()
			return nil, err
		}

		// Wrap the client stream to end the span when the stream is closed
		return &wrappedClientStream{ClientStream: clientStream, span: span}, nil
	}
}

type wrappedClientStream struct {
	grpc.ClientStream
	span trace.Span
}

func (w *wrappedClientStream) CloseSend() error {
	err := w.ClientStream.CloseSend()
	w.span.End()
	return err
}
