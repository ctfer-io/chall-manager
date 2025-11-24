package server

import (
	"context"
	"errors"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// statusFromError normalizes internal errors into gRPC status errors so the gateway
// can forward meaningful codes/messages to HTTP clients.
func statusFromError(err error) error {
	if err == nil {
		return nil
	}

	// If already a status error with a meaningful code, keep it.
	if s, ok := status.FromError(err); ok && s.Code() != codes.Unknown {
		return err
	}

	switch e := err.(type) {
	case *errs.ErrChallengeExist:
		if e.Exist {
			return status.Error(codes.AlreadyExists, e.Error())
		}
		return status.Error(codes.NotFound, e.Error())
	case *errs.ErrInstanceExist:
		if e.Exist {
			return status.Error(codes.AlreadyExists, e.Error())
		}
		return status.Error(codes.NotFound, e.Error())
	case *errs.ErrScenario:
		return status.Error(codes.InvalidArgument, e.Error())
	case *errs.ErrInternal:
		return status.Error(codes.Internal, e.Error())
	case errs.ErrProvisioningFailed:
		return status.Error(codes.FailedPrecondition, e.Error())
	case errs.ErrValidationFailed:
		return status.Error(codes.InvalidArgument, e.Error())
	}

	switch {
	case errors.Is(err, errs.ErrPulumiCanceled):
		return status.Error(codes.Aborted, errs.ErrPulumiCanceled.Error())
	case errors.Is(err, errs.ErrScenarioNoSub):
		return status.Error(codes.InvalidArgument, errs.ErrScenarioNoSub.Error())
	case errors.Is(err, errs.ErrInternalNoSub):
		return status.Error(codes.Internal, errs.ErrInternalNoSub.Error())
	case errors.Is(err, errs.ErrChallengeExpired):
		return status.Error(codes.FailedPrecondition, errs.ErrChallengeExpired.Error())
	case errors.Is(err, errs.ErrRenewNotAllowed):
		return status.Error(codes.FailedPrecondition, errs.ErrRenewNotAllowed.Error())
	case errors.Is(err, errs.ErrInstanceExpiredRenew):
		return status.Error(codes.FailedPrecondition, errs.ErrInstanceExpiredRenew.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	}

	return status.Error(codes.Unknown, err.Error())
}

// errorUnaryInterceptor converts returned errors into gRPC status errors so that the
// gRPC-Gateway can translate them into proper HTTP codes/messages.
func errorUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	resp, err = handler(ctx, req)
	if err != nil {
		err = statusFromError(err)
	}
	return resp, err
}
