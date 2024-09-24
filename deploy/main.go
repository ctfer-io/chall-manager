package main

import (
	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "chall-manager")

		// Create the namespace, but is not expected to run so in production.
		ns, err := corev1.NewNamespace(ctx, "deploy-namespace", &corev1.NamespaceArgs{
			Metadata: v1.ObjectMetaArgs{
				Name: pulumi.String(cfg.Get("namespace")),
			},
		})
		if err != nil {
			return nil
		}

		// Deploy the Chall-Manager service.
		cm, err := services.NewChallManager(ctx, ctx.Stack(), &services.ChallManagerArgs{
			Namespace:    ns.Metadata.Name().Elem(),
			Tag:          pulumi.String(cfg.Get("tag")),
			EtcdReplicas: pulumi.Int(cfg.GetInt("etcd.replicas")),
			Replicas:     pulumi.Int(cfg.GetInt("replicas")),
			JanitorCron:  pulumi.String(cfg.Get("janitor.cron")),
			Gateway:      cfg.GetBool("gateway"),
			Swagger:      cfg.GetBool("swagger"),
			Otel:         otelArgs(ctx, cfg),
		})
		if err != nil {
			return err
		}

		ctx.Export("endpoint-grpc", cm.EndpointGrpc)
		ctx.Export("endpoint-rest", cm.EndpointRest)

		return nil
	})
}

func otelArgs(ctx *pulumi.Context, cfg *config.Config) *common.OtelArgs {
	// Require "otel.endpoint" to turn it on
	if edp, err := cfg.Try("otel.endpoint"); err != nil || edp == "" {
		return nil
	}
	return &common.OtelArgs{
		ServiceName: pulumi.String(ctx.Stack()),
		Endpoint:    pulumi.String(cfg.Get("otel.endpoint")),
		Insecure:    cfg.GetBool("otel.insecure"),
	}
}
