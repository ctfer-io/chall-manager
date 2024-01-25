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
		})
		if err != nil {
			return err
		}

		ctx.Export("port", cm.Port)

		return nil
	})
}
