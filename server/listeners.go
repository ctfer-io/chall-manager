package server

import (
	"net"

	"go.uber.org/multierr"
	"google.golang.org/grpc"
)

// Listeners combines the API server listeners and forwarders.
type Listeners struct {
	// Main handle all incoming requests.
	Main net.Listener

	// GWConn forwards HTTP requests to the gRPC server.
	GWConn *grpc.ClientConn
}

// Close all underlying connections (i.e. listeners and forwarders).
func (l *Listeners) Close() error {
	return multierr.Combine(
		l.Main.Close(),
		l.GWConn.Close(),
	)
}
