package main

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Create a namespace
		ns, err := corev1.NewNamespace(ctx, "sa-ns-test", &corev1.NamespaceArgs{})
		if err != nil {
			return err
		}

		ctx.Export("flags", pulumi.StringArray{
			ns.Metadata.Name().Elem(),
		})
		ctx.Export("connection_info", pulumi.Sprintf("... -n %s", ns.Metadata.Name().Elem()))

		return nil
	})
}
