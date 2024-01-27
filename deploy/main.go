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
			Namespace:   pulumi.String(cfg.Get("namespace")),
			ServiceType: pulumi.String(cfg.Get("service-type")),
			Gateway:     toBool(cfg.Get("gateway")),
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
	case "false":
		return false
	}
	panic("invalid bool value: " + str)
}
