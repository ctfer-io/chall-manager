package main

import (
	"strconv"

	"github.com/ctfer-io/chall-manager/deploy/components"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "chall-manager")

		ns, err := corev1.NewNamespace(ctx, "deploy-namespace", &corev1.NamespaceArgs{
			Metadata: v1.ObjectMetaArgs{
				Name: pulumi.String(cfg.Get("namespace")),
			},
		})
		if err != nil {
			return nil
		}

		cm, err := components.NewChallManager(ctx, &components.ChallManagerArgs{
			Namespace:    ns.Metadata.Name().Elem(),
			ServiceType:  pulumi.String(cfg.Get("service-type")),
			Replicas:     toIntPtr(cfg.Get("replicas")),
			JanitorCron:  toStr(cfg, "janitor-cron"),
			Gateway:      toBool(cfg.Get("gateway")),
			Swagger:      toBool(cfg.Get("swagger")),
			LockKind:     cfg.Get("lock-kind"),
			EtcdReplicas: toIntPtr(cfg.Get("etcd-replicas")),
		})
		if err != nil {
			return err
		}

		ctx.Export("port", cm.Port)
		ctx.Export("gw-port", cm.GatewayPort)

		return nil
	})
}

func toBool(str string) bool {
	switch str {
	case "true":
		return true
	case "", "false":
		return false
	}
	panic("invalid bool value: " + str)
}

func toIntPtr(str string) pulumi.IntPtrInput {
	if str == "" {
		return nil
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		panic(err)
	}
	return pulumi.IntPtr(n)
}

func toStr(cfg *config.Config, key string) pulumi.StringInput {
	if _, err := cfg.Try(key); err != nil {
		return nil
	}
	return pulumi.String(cfg.Get(key))
}
