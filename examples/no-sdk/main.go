package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// 1. Load config
		cfg := config.New(ctx, "no-sdk")
		config := map[string]string{
			"identity": cfg.Get("identity"),
		}

		// 2. Create resources
		// ...

		// 3. Export outputs
		ctx.Export("connection_info", pulumi.Sprintf("curl -v https://%s.brefctf.ctfer.io", config["identity"]))
		return nil
	})
}
