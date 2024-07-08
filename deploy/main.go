package main

import (
	"github.com/ctfer-io/chall-manager/deploy/components"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "chall-manager")

		cm, err := components.NewChallManager(ctx, &components.ChallManagerArgs{
			Namespace:    toStr(cfg, "namespace"),
			ServiceType:  toStr(cfg, "service-type"),
			EtcdReplicas: pulumi.IntPtr(1), // XXX does not work properly, nil pointer dereference
			Replicas:     pulumi.IntPtr(1), // XXX does not work properly, nil pointer dereference
			JanitorCron:  toStr(cfg, "janitor-cron"),
			Gateway:      toBool(cfg.Get("gateway")),
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
	case "false", "":
		return false
	}
	panic("invalid bool value: " + str)
}

func toStr(cfg *config.Config, key string) pulumi.StringInput {
	if _, err := cfg.Try(key); err != nil {
		return nil
	}
	return pulumi.String(cfg.Get(key))
}
