package main

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := loadConfig(ctx)

		// Create the namespace, but is not expected to run so in production.
		ns, err := corev1.NewNamespace(ctx, "deploy-namespace", &corev1.NamespaceArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.String(cfg.Namespace),
			},
		})
		if err != nil {
			return nil
		}

		// Deploy the Chall-Manager service.
		args := &services.ChallManagerArgs{
			Namespace:       ns.Metadata.Name().Elem(),
			Tag:             pulumi.String(cfg.Tag),
			PrivateRegistry: pulumi.String(cfg.PrivateRegistry),
			Replicas:        pulumi.Int(cfg.Replicas),
			Swagger:         cfg.Swagger,
			EtcdReplicas:    nil,
			JanitorCron:     nil,
			Otel:            nil,
		}
		if cfg.Etcd != nil {
			args.EtcdReplicas = pulumi.IntPtr(cfg.Etcd.Replicas)
		}
		if cfg.Janitor != nil {
			args.JanitorMode = parts.JanitorMode(cfg.Janitor.Mode)
			args.JanitorCron = pulumi.StringPtrFromPtr(cfg.Janitor.Cron)
			args.JanitorTicker = pulumi.StringPtrFromPtr(cfg.Janitor.Ticker)
		}
		if cfg.Otel != nil {
			args.Otel = &common.OtelArgs{
				ServiceName: pulumi.String(ctx.Stack()),
				Endpoint:    pulumi.String(cfg.Otel.Endpoint),
				Insecure:    cfg.Otel.Insecure,
			}
		}
		cm, err := services.NewChallManager(ctx, ctx.Stack(), args)
		if err != nil {
			return err
		}

		ctx.Export("endpoint", cm.Endpoint)

		return nil
	})
}

type (
	Config struct {
		Namespace       string         `json:"namespace"`
		Tag             string         `json:"tag"`
		PrivateRegistry string         `json:"private-registry"`
		Etcd            *EtcdConfig    `json:"etcd"`
		Replicas        int            `json:"replicas"`
		Janitor         *JanitorConfig `json:"janitor"`
		Swagger         bool           `json:"swagger"`
		Otel            *OtelConfig    `json:"otel"`
	}

	EtcdConfig struct {
		Replicas int `json:"replicas"`
	}

	JanitorConfig struct {
		Cron   *string `json:"cron,omitempty"`
		Ticker *string `json:"ticker,omitempty"`
		Mode   string  `json:"mode"`
	}

	OtelConfig struct {
		Endpoint string `json:"endpoint"`
		Insecure bool   `json:"insecure"`
	}
)

func loadConfig(ctx *pulumi.Context) *Config {
	cfg := config.New(ctx, "")
	c := &Config{
		Namespace:       cfg.Get("namespace"),
		Tag:             cfg.Get("tag"),
		PrivateRegistry: cfg.Get("private-registry"),
		Replicas:        cfg.GetInt("replicas"),
		Swagger:         cfg.GetBool("swagger"),
	}

	var etcdC EtcdConfig
	if err := cfg.TryObject("etcd", &etcdC); err == nil && etcdC.Replicas != 0 {
		c.Etcd = &etcdC
	}

	var janitorC JanitorConfig
	if err := cfg.TryObject("janitor", &janitorC); err == nil {
		c.Janitor = &janitorC
	}

	var otelC OtelConfig
	if err := cfg.TryObject("otel", &otelC); err == nil && otelC.Endpoint != "" {
		c.Otel = &otelC
	}

	return c
}
