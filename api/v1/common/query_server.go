package common

import (
	"sync"

	"google.golang.org/grpc"
)

// QueryServer is a generic implementation of a synchronous gRPC server.
// Use it when dealing with a query that streams asynchronously, but gRPC
// requires it to be else it will crash the stream.
type QueryServer[T any] struct {
	srv grpc.ServerStream
	mx  *sync.Mutex
}

// NewQueryServer creates a throwable server to stream synchronously to
// a gRPC stream server.
func NewQueryServer[T any](server grpc.ServerStream) *QueryServer[T] {
	return &QueryServer[T]{
		srv: server,
		mx:  &sync.Mutex{},
	}
}

// SendMsg synchronously when asynchrouneous tasks is possible.
func (qs *QueryServer[T]) SendMsg(in T) error {
	qs.mx.Lock()
	defer qs.mx.Unlock()

	return qs.srv.SendMsg(in)
}
