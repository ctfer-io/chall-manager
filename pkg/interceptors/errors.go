package interceptors

import (
	"context"

	"google.golang.org/grpc"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// UnaryClientErrors converts returned errors into gRPC status errors so that the
// gRPC-Gateway can translate them into proper HTTP codes/messages.
func UnaryClientErrors(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	resp, err = handler(ctx, req)
	if err != nil {
		err = errs.StatusFromError(err)
	}
	return resp, err
}

// StreamServerErrors converts returned errors into gRPC status errors so that
// gRPC-Gatewat can translate them into proper HTTP codes/messages.
func StreamServerErrors(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	err := handler(srv, &wrappedServerStream{ServerStream: ss})
	if err != nil {
		err = errs.StatusFromError(err)
	}
	return err
}

type wrappedServerStream struct {
	grpc.ServerStream
}

func (w *wrappedServerStream) SendMsg(m any) error {
	if err := w.ServerStream.SendMsg(m); err != nil {
		return errs.StatusFromError(err)
	}
	return nil
}

func (w *wrappedServerStream) RecvMsg(m any) error {
	if err := w.ServerStream.RecvMsg(m); err != nil {
		return errs.StatusFromError(err)
	}
	return nil
}
