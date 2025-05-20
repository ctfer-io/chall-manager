package main

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := loadConfig(ctx)

		// Upsert namespace if it is not configured (imply it was already created)
		namespace := pulumi.String(cfg.Namespace).ToStringOutput()
		if cfg.Namespace == "" {
			ns, err := corev1.NewNamespace(ctx, "namespace", &corev1.NamespaceArgs{})
			if err != nil {
				return nil
			}
			namespace = ns.Metadata.Name().Elem()
		}

		// Deploy the Chall-Manager service.
		args := &services.ChallManagerArgs{
			Namespace: namespace,
			Tag:       pulumi.String(cfg.Tag),
			Registry:  pulumi.String(cfg.Registry),
			Replicas:  pulumi.Int(cfg.Replicas),
			Swagger:   cfg.Swagger,
			PVCAccessModes: pulumi.ToStringArray([]string{
				cfg.PVCAccessMode,
			}),
			PVCStorageSize: pulumi.String(cfg.PVCStorageSize),
			Expose:         cfg.Expose,
			RomeoClaimName: pulumi.String(cfg.RomeoClaimName),
			Kubeconfig:     cfg.Kubeconfig,
			Requests:       pulumi.ToStringMap(cfg.Requests),
			Limits:         pulumi.ToStringMap(cfg.Limits),
		}
		if cfg.Etcd != nil {
			args.EtcdReplicas = pulumi.IntPtr(cfg.Etcd.Replicas)
		}
		if cfg.Janitor != nil {
			args.JanitorMode = parts.JanitorMode(cfg.Janitor.Mode)
			args.JanitorCron = pulumi.String(cfg.Janitor.Cron)
			args.JanitorTicker = pulumi.String(cfg.Janitor.Ticker)
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
		ctx.Export("exposed_port", cm.ExposedPort)

		return nil
	})
}

type (
	Config struct {
		Namespace      string
		Tag            string
		Registry       string
		Etcd           *EtcdConfig
		Replicas       int
		Janitor        *JanitorConfig
		Swagger        bool
		PVCAccessMode  string
		PVCStorageSize string
		Expose         bool
		LogLevel       string
		RomeoClaimName string
		Requests       map[string]string
		Limits         map[string]string
		Otel           *OtelConfig

		// Secrets

		Kubeconfig pulumi.StringOutput
	}

	EtcdConfig struct {
		Replicas int `json:"replicas"`
	}

	JanitorConfig struct {
		Cron   string `json:"cron"`
		Ticker string `json:"ticker"`
		Mode   string `json:"mode"`
	}

	OtelConfig struct {
		Endpoint string `json:"endpoint"`
		Insecure bool   `json:"insecure"`
	}
)

func loadConfig(ctx *pulumi.Context) *Config {
	cfg := config.New(ctx, "")
	c := &Config{
		Namespace:      cfg.Get("namespace"),
		Tag:            cfg.Get("tag"),
		Registry:       cfg.Get("registry"),
		LogLevel:       cfg.Get("log-level"),
		Replicas:       cfg.GetInt("replicas"),
		Swagger:        cfg.GetBool("swagger"),
		PVCAccessMode:  cfg.Get("pvc-access-mode"),
		PVCStorageSize: cfg.Get("pvc-storage-size"),
		Expose:         cfg.GetBool("expose"),
		RomeoClaimName: cfg.Get("romeo-claim-name"),
		Kubeconfig:     cfg.GetSecret("kubeconfig"),
	}

	if err := cfg.TryObject("requests", &c.Requests); err != nil {
		panic(err)
	}

	if err := cfg.TryObject("limits", &c.Limits); err != nil {
		panic(err)
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
