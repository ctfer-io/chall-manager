package main

import (
	"context"
	"net/mail"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/server"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
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

func main() {
	cmd := &cli.Command{
		Name:  "Chall-Manager",
		Usage: "Challenge Instances on Demand, anywhere at any time",
		Flags: []cli.Flag{
			cli.VersionFlag,
			cli.HelpFlag,
			&cli.IntFlag{
				Name:     "port",
				Aliases:  []string{"p"},
				Sources:  cli.EnvVars("PORT"),
				Category: "global",
				Value:    8080,
				Usage:    "Define the API server port to listen on (gRPC+HTTP).",
			},
			&cli.BoolFlag{
				Name:     "swagger",
				Sources:  cli.EnvVars("SWAGGER"),
				Category: "global",
				Value:    false,
				Usage:    "If set, turns on the API gateway swagger on `/swagger`.",
			},
			&cli.StringFlag{
				Name:        "dir",
				Aliases:     []string{"d"},
				Sources:     cli.EnvVars("DIR"),
				Category:    "global",
				Value:       "/tmp/chall-manager",
				Destination: &global.Conf.Directory,
				Usage:       "Define the volume to read/write stack and states to. It should be sharded across replicas for HA.",
			},
			&cli.StringFlag{
				Name:     "log-level",
				Sources:  cli.EnvVars("LOG_LEVEL"),
				Category: "global",
				Value:    "info",
				Action: func(_ context.Context, _ *cli.Command, lvl string) error {
					_, err := zapcore.ParseLevel(lvl)
					return err
				},
				Destination: &global.Conf.LogLevel,
				Usage:       "Use to specify the level of logging.",
			},
			&cli.BoolFlag{
				Name:        "tracing",
				Sources:     cli.EnvVars("TRACING"),
				Category:    "otel",
				Destination: &global.Conf.Otel.Tracing,
				Usage:       "If set, turns on tracing through OpenTelemetry (see https://opentelemetry.io for more info).",
			},
			&cli.StringFlag{
				Name:        "service-name",
				Sources:     cli.EnvVars("OTEL_SERVICE_NAME"),
				Category:    "otel",
				Value:       "chall-manager",
				Destination: &global.Conf.Otel.ServiceName,
				Usage:       "Override the service name. Useful when deploying multiple instances to filter signals.",
			},
			&cli.StringFlag{
				Name:        "etcd.endpoint",
				Sources:     cli.EnvVars("ETCD_ENDPOINT"),
				Category:    "lock",
				Usage:       "Define the etcd endpoints to reach for locks.",
				Destination: &global.Conf.Etcd.Endpoint,
			},
			&cli.StringFlag{
				Name:        "etcd.username",
				Sources:     cli.EnvVars("ETCD_USERNAME"),
				Category:    "lock",
				Destination: &global.Conf.Etcd.Username,
				Usage:       "If lock is etcd, define the username to use to connect to the etcd cluster.",
				Action: func(_ context.Context, cmd *cli.Command, _ string) error {
					if cmd.String("etcd.endpoint") == "" {
						return errors.New("must configure an etcd endpoint along credentials")
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "etcd.password",
				Sources:     cli.EnvVars("ETCD_PASSWORD"),
				Category:    "lock",
				Destination: &global.Conf.Etcd.Password,
				Usage:       "If lock is etcd, define the password to use to connect to the etcd cluster.",
				Action: func(_ context.Context, cmd *cli.Command, _ string) error {
					if cmd.String("etcd.endpoint") == "" {
						return errors.New("must configure an etcd endpoint along credentials")
					}
					return nil
				},
			},
			&cli.BoolFlag{
				Name:        "oci.insecure",
				Sources:     cli.EnvVars("OCI_INSECURE"),
				Category:    "scenario",
				Destination: &global.Conf.OCI.Insecure,
				Usage:       "If set to true, use HTTP rather than HTTPS.",
			},
			&cli.StringFlag{
				Name:        "oci.username",
				Sources:     cli.EnvVars("OCI_USERNAME"),
				Category:    "scenario",
				Destination: global.Conf.OCI.Username,
				Usage:       `Configure the OCI registry username to pull scenarios from.`,
			},
			&cli.StringFlag{
				Name:        "oci.password",
				Sources:     cli.EnvVars("OCI_PASSWORD"),
				Category:    "scenario",
				Destination: global.Conf.OCI.Password,
				Usage:       `Configure the OCI registry password to pull scenarios from.`,
			},
		},
		Action: run,
		Authors: []any{
			mail.Address{
				Name:    "Lucas Tesson - PandatiX",
				Address: "lucastesson@protonmail.com",
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

	ctx := context.Background()
	if err := cmd.Run(ctx, os.Args); err != nil {
		global.Log().Error(ctx, "fatal error",
			zap.Error(err),
		)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	// Pre-flight global configuration
	global.Version = version

	port := cmd.Int("port")
	sw := cmd.Bool("swagger")
	tracing := cmd.Bool("tracing")

	// Initialize tracing and handle the tracer provider shutdown
	if tracing {
		// Set up OpenTelemetry.
		otelShutdown, err := global.SetupOtelSDK(ctx)
		if err != nil {
			return err
		}
		// Handle shutdown properly so nothing leaks.
		defer func() {
			err = multierr.Append(err, otelShutdown(ctx))
		}()
	}

	logger := global.Log()
	logger.Info(ctx, "starting API server",
		zap.Int("port", port),
		zap.Bool("swagger", sw),
		zap.String("directory", global.Conf.Directory),
		zap.Bool("tracing", tracing),
	)

	// Create context that listens for the interrupt signal from the OS
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
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

	if edp := cmd.String("etcd.endpoint"); edp != "" {
		ctx = context.WithoutCancel(ctx)
		if err := global.GetEtcdManager().Close(ctx); err != nil {
			logger.Error(ctx, "closing connection to etcd",
				zap.Error(err),
			)
		}
	}

	return nil
}
