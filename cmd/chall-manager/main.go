package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/server"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
	builtBy = ""
)

const (
	etcdKey = "etcd"
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
				Usage:    "Define the API server port to listen on (gRPC+HTTP).",
			},
			&cli.BoolFlag{
				Name:     "swagger",
				EnvVars:  []string{"SWAGGER"},
				Category: "global",
				Value:    false,
				Usage:    "If set, turns on the API gateway swagger on `/swagger`.",
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
				Name:     "log-level",
				EnvVars:  []string{"LOG_LEVEL"},
				Category: "global",
				Value:    "info",
				Action: func(_ *cli.Context, lvl string) error {
					_, err := zapcore.ParseLevel(lvl)
					return err
				},
				Destination: &global.Conf.LogLevel,
				Usage:       "Use to specify the level of logging.",
			},
			&cli.BoolFlag{
				Name:        "tracing",
				EnvVars:     []string{"TRACING"},
				Category:    "otel",
				Destination: &global.Conf.Otel.Tracing,
				Usage:       "If set, turns on tracing through OpenTelemetry (see https://opentelemetry.io for more info).",
			},
			&cli.StringFlag{
				Name:        "service-name",
				EnvVars:     []string{"OTEL_SERVICE_NAME"},
				Category:    "otel",
				Value:       "chall-manager",
				Destination: &global.Conf.Otel.ServiceName,
				Usage:       "Override the service name. Useful when deploying multiple instances to filter signals.",
			},
			&cli.StringFlag{
				Name:        "lock-kind",
				EnvVars:     []string{"LOCK_KIND"},
				Category:    "lock",
				Value:       "local",
				Destination: &global.Conf.Lock.Kind,
				Usage: `Define the lock kind to use. ` +
					`It could either be "ectd" for Kubernetes-native deployments (recommended) or "local" for an ` +
					`instance-only lock (not scalable).`,
				Action: func(_ *cli.Context, s string) error {
					if !slices.Contains([]string{etcdKey, "local"}, s) {
						return errors.New("invalid lock kind value")
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:     "lock-etcd-endpoint",
				EnvVars:  []string{"LOCK_ETCD_ENDPOINT"},
				Category: "lock",
				Usage:    "Define the etcd endpoints to reach for locks.",
				Action: func(ctx *cli.Context, s string) error {
					if ctx.String("lock-kind") != etcdKey {
						return errors.New("incompatible lock kind with lock-etcd-endpoints, expect etcd")
					}

					// use action instead of destination to avoid dealing with conversions
					global.Conf.Lock.EtcdEndpoint = s
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "lock-etcd-username",
				EnvVars:     []string{"LOCK_ETCD_USERNAME"},
				Category:    "lock",
				Destination: &global.Conf.Lock.EtcdUsername,
				Usage:       "If lock kind is etcd, define the username to use to connect to the etcd cluster.",
				Action: func(ctx *cli.Context, _ string) error {
					if ctx.String("lock-kind") != etcdKey {
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
				Action: func(ctx *cli.Context, _ string) error {
					if ctx.String("lock-kind") != "etcd" {
						return errors.New("incompatible lock kind with lock-etcd-password, expect etcd")
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "oci-registry-username",
				EnvVars:     []string{"OCI_REGISTRY_USERNAME"},
				Category:    "scenario",
				Destination: global.Conf.OCI.Username,
				Usage:       `Configure the OCI registry username to pull scenarios from.`,
			},
			&cli.StringFlag{
				Name:        "oci-registry-password",
				EnvVars:     []string{"OCI_REGISTRY_PASSWORD"},
				Category:    "scenario",
				Destination: global.Conf.OCI.Password,
				Usage:       `Configure the OCI registry password to pull scenarios from.`,
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
	// Pre-flight global configuration
	global.Version = version

	port := c.Int("port")
	sw := c.Bool("swagger")
	tracing := c.Bool("tracing")

	// Initialize tracing and handle the tracer provider shutdown
	if tracing {
		// Set up OpenTelemetry.
		otelShutdown, err := global.SetupOtelSDK(c.Context)
		if err != nil {
			return err
		}
		// Handle shutdown properly so nothing leaks.
		defer func() {
			err = multierr.Append(err, otelShutdown(c.Context))
		}()
	}

	logger := global.Log()
	logger.Info(c.Context, "starting API server",
		zap.Int("port", port),
		zap.Bool("swagger", sw),
		zap.String("directory", global.Conf.Directory),
		zap.Bool("tracing", tracing),
	)

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

	// Launch API server
	srv := server.NewServer(server.Options{
		Port:    port,
		Swagger: sw,
	})
	if err := srv.Run(ctx); err != nil {
		return err
	}

	// Listen for the interrupt signal
	<-ctx.Done()

	// Restore default behavior on the interrupt signal
	stop()
	logger.Info(ctx, "shutting down gracefully")

	if c.String("lock-kind") == etcdKey {
		ctx = context.WithoutCancel(ctx)
		if err := global.GetEtcdManager().Close(ctx); err != nil {
			logger.Error(ctx, "closing connection to etcd",
				zap.Error(err),
			)
		}
	}

	return nil
}
