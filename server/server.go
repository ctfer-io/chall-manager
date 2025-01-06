package server

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/soheilhy/cmux"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
)

// Server is a helper to manager an API Server.
type Server struct {
	Options

	lns *Listeners
}

// Options to configure it once for all.
type Options struct {
	Port    int
	Swagger bool
}

// NewServer returns a fresh API server.
func NewServer(opts Options) *Server {
	return &Server{
		Options: opts,
	}
}

// Run the API server in backend.
// It first start the listeners then proceed to launch the connection
// multiplexers for the gRPC server and its HTTP gateway.
func (s *Server) Run(ctx context.Context) (err error) {
	err = s.listen(ctx)
	if err != nil {
		return err
	}

	// Create servers
	grpcServer := s.newGRPCServer()
	grpcWebServer := grpcweb.WrapServer(grpcServer)
	httpServer := s.newHTTPServer(ctx, grpcWebServer)

	// Build a multiplexer to handle gRPC or HTTP services
	tcpm := cmux.New(s.lns.Main)
	httpL := tcpm.Match( // all HTTP methods used in the API v1
		cmux.HTTP1Fast(http.MethodGet),
		cmux.HTTP1Fast(http.MethodPost),
		cmux.HTTP1Fast(http.MethodPatch),
		cmux.HTTP1Fast(http.MethodDelete),
	)
	grpcL := tcpm.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))

	// Start servicing
	logger := global.Log()
	go func() {
		if err := grpcServer.Serve(grpcL); err != nil {
			logger.Error(ctx, "grpc server", zap.Error(err))
		}
	}()
	go func() {
		if err := httpServer.Serve(httpL); err != nil {
			logger.Error(ctx, "http server", zap.Error(err))
		}
	}()
	go func() {
		if err := tcpm.Serve(); err != nil {
			logger.Error(ctx, "cmux", zap.Error(err))
		}
	}()

	return nil
}

func (s *Server) listen(ctx context.Context) error {
	// Initiate TCP listener (overall API server listener)
	global.Log().Info(ctx, "api-server start listening",
		zap.Int("port", s.Port),
	)
	mainL, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return err
	}

	// Create HTTP->gRPC forwarder
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt)),
		// TODO add OpenTelemetry interceptors
		// grpc.WithUnaryInterceptor(OTELUnaryClientInterceptor()),
		// grpc.WithStreamInterceptor(OTELStreamClientInterceptor()),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", s.Port), opts...)
	if err != nil {
		return multierr.Combine(err, mainL.Close())
	}

	s.lns = &Listeners{
		Main:   mainL,
		GWConn: conn,
	}
	return nil
}

func (s *Server) newGRPCServer() *grpc.Server {
	// Create the gRPC server
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(math.MaxInt64),
		grpc.MaxSendMsgSize(math.MaxInt64),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
			// XXX remove deprecation notice by fixing this shit
			otelgrpc.StreamServerInterceptor(),
		)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			// XXX remove deprecation notice by fixing this shit
			otelgrpc.UnaryServerInterceptor(),
		)),
	}
	grpcServer := grpc.NewServer(opts...)

	// Register every services
	challenge.RegisterChallengeStoreServer(grpcServer, challenge.NewStore())
	instance.RegisterInstanceManagerServer(grpcServer, instance.NewManager())

	return grpcServer
}

func (s *Server) newHTTPServer(ctx context.Context, grpcWebHandler http.Handler) *http.Server {
	// Create multiplexer and register it in an HTTP server
	mux := http.NewServeMux()
	httpServer := http.Server{
		Addr: fmt.Sprintf("localhost:%d", s.Port),
		Handler: &handlerSwitcher{
			handler: mux,
			contentTypeToHandler: map[string]http.Handler{
				"application/grpc-web+proto": grpcWebHandler,
			},
		},
		ReadHeaderTimeout: time.Second,
	}

	// Build gateway to the HTTP 1.1+JSON server
	gwmux := runtime.NewServeMux()

	mux.Handle("/api/v1/", gwmux)

	// Add swagger if requested
	if s.Swagger {
		addSwagger(mux)
	}

	// Register all HTTP->gRPC forwarders
	must(challenge.RegisterChallengeStoreHandler(ctx, gwmux, s.lns.GWConn))
	must(instance.RegisterInstanceManagerHandler(ctx, gwmux, s.lns.GWConn))

	return &httpServer
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
