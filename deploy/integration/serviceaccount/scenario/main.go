package main

import (
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Create a namespace
		ns, err := parts.NewNamespace(ctx, "ns", &parts.NamespaceArgs{
			Name: pulumi.String("sa-ns-test"),
		})
		if err != nil {
			return err
		}

		ctx.Export("flags", pulumi.StringArray{
			ns.Name,
		})
		ctx.Export("connection_info", pulumi.Sprintf("... -n %s", ns.Name))

		return nil
	})
}
