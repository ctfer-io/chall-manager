package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/interceptors"
	sw "github.com/ctfer-io/chall-manager/pkg/swagger"
	swagger "github.com/ctfer-io/chall-manager/swagger-ui"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
	builtBy = ""
)

func main() {
	app := &cli.App{
		Name:  "Chall-Manager",
		Usage: "Chall-Manager is a Kubernetes-native μService that deploys challenge scenario on demand, powered by Pulumi.",
		Flags: []cli.Flag{
			cli.VersionFlag,
			cli.HelpFlag,
			&cli.IntFlag{
				Name:     "port",
				Aliases:  []string{"p"},
				EnvVars:  []string{"PORT"},
				Category: "global",
				Value:    8080,
				Usage:    "Define the gRPC server port to listen on.",
			},
			&cli.BoolFlag{
				Name:     "gw",
				EnvVars:  []string{"GATEWAY"},
				Category: "global",
				Value:    false,
				Usage:    "If set, turns on the gateway.",
			},
			&cli.IntFlag{
				Name:     "gw-port",
				EnvVars:  []string{"GATEWAY_PORT"},
				Category: "global",
				Value:    9090,
				Usage:    "Define the REST API (gRPC API gateway) server port to listen on.",
			},
			&cli.BoolFlag{
				Name:     "gw-swagger",
				EnvVars:  []string{"GATEWAY_SWAGGER"},
				Category: "global",
				Value:    false,
				Usage:    "If set, turns on the gateway swagger on /swagger.",
			},
			&cli.StringFlag{
				Name:        "dir",
				Aliases:     []string{"d"},
				EnvVars:     []string{"DIR"},
				Category:    "global",
				Value:       "/tmp/chall-manager",
				Destination: &global.Conf.Directory,
				Usage:       "Define the volume to read/write stack and states to. It should be sharded across replicas for HA.",
			},
			&cli.StringFlag{
				Name:        "salt",
				Aliases:     []string{"s"},
				EnvVars:     []string{"SALT"},
				Category:    "global",
				Value:       "",
				Destination: &global.Conf.Salt,
				Usage:       "Define the salt to use for generating the identity. We recommend setting a random one.",
			},
			&cli.BoolFlag{
				Name:     "tracing",
				EnvVars:  []string{"TRACING"},
				Category: "global",
				Usage:    "If set, turns on tracing through OpenTelemetry (see https://opentelemetry.io) for more info.",
			},
			&cli.StringFlag{
				Name:        "lock-kind",
				EnvVars:     []string{"LOCK_KIND"},
				Category:    "lock",
				Value:       "etcd",
				Destination: &global.Conf.Lock.Kind,
				Usage:       "Define the lock kind to use. It could either be \"ectd\" for Kubernetes-native deployments (recommended) or \"local\" for an instance-only lock (not scalable).",
				Action: func(ctx *cli.Context, s string) error {
					if !slices.Contains([]string{"etcd", "local"}, s) {
						return errors.New("invalid lock kind value")
					}
					return nil
				},
			},
			&cli.StringSliceFlag{
				Name:     "lock-etcd-endpoints",
				EnvVars:  []string{"LOCK_ETCD_ENDPOINTS"},
				Category: "lock",
				Usage:    "Define the etcd endpoints to reach for locks.",
				Action: func(ctx *cli.Context, s []string) error {
					if ctx.String("lock-kind") != "etcd" {
						return errors.New("incompatible lock kind with lock-etcd-endpoints, expect etcd")
					}

					// use action instead of destination to avoid dealing with conversions
					global.Conf.Lock.EtcdEndpoints = s
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "lock-etcd-username",
				EnvVars:     []string{"LOCK_ETCD_USERNAME"},
				Category:    "lock",
				Destination: &global.Conf.Lock.EtcdUsername,
				Usage:       "If lock kind is etcd, define the username to use to connect to the etcd cluster.",
				Action: func(ctx *cli.Context, s string) error {
					if ctx.String("lock-kind") != "etcd" {
						return errors.New("incompatible lock kind with lock-etcd-username, expect etcd")
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "lock-etcd-password",
				EnvVars:     []string{"LOCK_ETCD_PASSWORD"},
				Category:    "lock",
				Destination: &global.Conf.Lock.EtcdPassword,
				Usage:       "If lock kind is etcd, define the password to use to connect to the etcd cluster.",
				Action: func(ctx *cli.Context, s string) error {
					if ctx.String("lock-kind") != "etcd" {
						return errors.New("incompatible lock kind with lock-etcd-password, expect etcd")
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "oci-registry-url",
				EnvVars:     []string{"OCI_REGISTRY_URL"},
				Category:    "scenario",
				Destination: global.Conf.OCI.RegistryURL,
				Usage:       "Configure the Docker registry URL to use as part of the Challenge Scenario on Demand factory. If a variant of a Docker image is built, it will be pushed there.",
			},
			&cli.StringFlag{
				Name:        "oci-registry-username",
				EnvVars:     []string{"OCI_REGISTRY_USERNAME"},
				Category:    "scenario",
				Destination: global.Conf.OCI.Username,
				Usage:       "Configure the Docker registry username to use as part of the Challenge Scenario on Demand factory. If a variant of a Docker image is built, it will be pushed to the registry URL with this username.",
			},
			&cli.StringFlag{
				Name:        "oci-registry-password",
				EnvVars:     []string{"OCI_REGISTRY_PASSWORD"},
				Category:    "scenario",
				Destination: global.Conf.OCI.Password,
				Usage:       "Configure the Docker registry username to use as part of the Challenge Scenario on Demand factory. If a variant of a Docker image is built, it will be pushed to the registry URL with this password.",
			},
		},
		Action: run,
		Authors: []*cli.Author{
			{
				Name:  "Lucas Tesson - PandatiX",
				Email: "lucastesson@protonmail.com",
			},
		},
		Version: version,
		Metadata: map[string]any{
			"version": version,
			"commit":  commit,
			"date":    date,
			"builtBy": builtBy,
		},
	}

	if err := app.Run(os.Args); err != nil {
		global.Log().Error(context.Background(), "fatal error",
			zap.Error(err),
		)
		os.Exit(1)
	}
}

func run(c *cli.Context) error {
	logger := global.Log()

	grpcPort := c.Int("port")
	gwPort := c.Int("gw-port")
	tracing := c.Bool("tracing")

	logger.Info(c.Context, "starting API servers",
		zap.Int("grpc", grpcPort),
		zap.Int("gw", gwPort),
		zap.Bool("gw_swagger", c.Bool("gw-swagger")),
		zap.String("directory", global.Conf.Directory),
		zap.Bool("tracing", tracing),
	)

	// Initialize tracing and handle the tracer provider shutdown
	if tracing {
		stopTracing := initTracing()
		defer stopTracing()
	}

	// Create context that listens for the interrupt signal from the OS
	ctx, stop := signal.NotifyContext(c.Context, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create temporary directory
	challDir := filepath.Join(global.Conf.Directory, "chall")
	if err := os.MkdirAll(challDir, os.ModePerm); err != nil {
		return errors.Wrapf(err, "during mkdir of challenges directory %s", challDir)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start gRPC server
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return err
	}
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			// Capture ingress/egress requests and log them
			logging.UnaryServerInterceptor(interceptors.InterceptorLogger(global.Log().Sub), logging.WithLogOnEvents(logging.StartCall, logging.FinishCall)),
		),
	}
	grpcServer := grpc.NewServer(opts...)
	challenge.RegisterChallengeStoreServer(grpcServer, challenge.NewStore())
	instance.RegisterInstanceManagerServer(grpcServer, instance.NewManager())
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error(ctx, "gRPC server stopped suddenly",
				zap.Error(err),
			)
			stop()
		}
	}()
	logger.Info(ctx, "gRPC server started")

	// Start REST API (gRPC gateway) if necessary
	var gwServer *http.Server
	if c.Bool("gw") {
		conn, err := grpc.NewClient(fmt.Sprintf(":%d", grpcPort),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return err
		}
		defer conn.Close()

		gwmux := runtime.NewServeMux()
		if merr := multierr.Combine(
			challenge.RegisterChallengeStoreHandlerClient(ctx, gwmux, challenge.NewChallengeStoreClient(conn)),
			instance.RegisterInstanceManagerHandlerClient(ctx, gwmux, instance.NewInstanceManagerClient(conn)),
		); merr != nil {
			return err
		}

		mux := http.NewServeMux()
		mux.Handle("/", gwmux)
		if c.Bool("gw-swagger") {
			mux.HandleFunc("/swagger/swagger.json", func(w http.ResponseWriter, r *http.Request) {
				swaggers := []string{
					"challenge",
					"instance",
					"common", // must be last to overwrite previous attributes
				}
				mergedSwagger := sw.NewMerger()
				for _, swagger := range swaggers {
					swaggerPath := fmt.Sprintf("./gen/api/v1/%[1]s/%[1]s.swagger.json", swagger)
					if err := mergedSwagger.AddFile(swaggerPath); err != nil {
						http.Error(w, "Merging swaggers", http.StatusInternalServerError)
						return
					}
				}
				b, err := mergedSwagger.MarshalJSON()
				if err != nil {
					http.Error(w, "Exporting merged swagger", http.StatusInternalServerError)
					return
				}
				if _, err := w.Write(b); err != nil {
					http.Error(w, "Writing  merged swagger", http.StatusInternalServerError)
					return
				}
			})
			mux.Handle("/swagger/", http.StripPrefix("/swagger/", http.FileServer(http.FS(swagger.Content))))
		}

		gwServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", gwPort),
			Handler: mux,
		}
		go func() {
			if err := gwServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error(ctx, "REST server stopped suddenly",
					zap.Error(err),
				)
				stop()
			}
		}()
		logger.Info(ctx, "REST server started")
	}

	// Listen for the interrupt signal
	<-ctx.Done()

	// Restore default behavior on the interrupt signal
	stop()
	logger.Info(ctx, "shutting down gracefully")

	grpcServer.GracefulStop()
	if gwServer != nil {
		if err := gwServer.Shutdown(ctx); err != nil {
			return errors.Wrap(err, "server forced to shutdown")
		}
	}

	logger.Info(ctx, "server exiting")
	return nil
}

func initTracing() func() {
	logger := global.Log()
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		logger.Error(context.Background(), "failed to create stdout exporter",
			zap.Error(err),
		)
		os.Exit(1)
	}

	// Create a simple span processor that writes to the exporter
	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(bsp))
	otel.SetTracerProvider(tp)

	// Set the global propagater to use W3C Trace Content
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Return a function to stop the tracer provider
	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			logger.Error(context.Background(), "failed to shut down trace provider",
				zap.Error(err),
			)
		}
	}
}
