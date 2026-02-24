package errors

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	st, err := status.New(codes.Internal, "An internal error occurred.").WithDetails(&errdetails.ErrorInfo{
		Reason: "INTERNAL_ERROR",
		Domain: Domain,
	})
	if err != nil {
		panic(err)
	}
	ErrInternalNoSub = st.Err()
}

var (
	ErrInternalNoSub error
	ErrCanceled      error = status.New(codes.Canceled, "").Err()
)
