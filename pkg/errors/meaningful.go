package errors

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type meaningfulError interface {
	statusError() error
}

// StatusFromError normalizes internal errors into gRPC status errors so the gateway
// can forward meaningful codes/messages to HTTP clients.
func StatusFromError(err error) error {
	if err == nil {
		return nil
	}

	// If already a status error with a meaningful code, keep it.
	if s, ok := status.FromError(err); ok && s.Code() != codes.Unknown {
		return err
	}

	// Else if an internal but meaningful error, get the status error
	if me, ok := err.(meaningfulError); ok {
		return me.statusError()
	}

	// Else wrap it in an unknown code, so that it appears in stats so will eventually get caught
	return status.Error(codes.Unknown, err.Error())
}
