package main

import (
	"github.com/ctfer-io/chall-manager/sdk"
	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	flag = "24HIUT{To0_W3ak_c#yp7o}"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		variated := pulumi.String(sdk.Variate(req.Config.Identity, flag,
			sdk.WithSpecial(true),
		))

		cm, err := k8s.NewExposedMonopod(req.Ctx, "test", &k8s.ExposedMonopodArgs{
			Identity: pulumi.String(req.Config.Identity),
			Hostname: pulumi.String("brefctf.ctfer.io"),
			Container: k8s.ContainerArgs{
				Image: pulumi.String("pandatix/license-lvl1:latest"),
				Ports: k8s.PortBindingArray{
					k8s.PortBindingArgs{
						Port:       pulumi.Int(8080),
						ExposeType: k8s.ExposeIngress,
					},
				},
				Files: pulumi.StringMap{
					"/app/flag.txt": variated,
				},
			},
			// The following fits for a Traefik-based use case
			IngressAnnotations: pulumi.ToStringMap(map[string]string{
				"traefik.ingress.kubernetes.io/router.entrypoints": "web, websecure",
			}),
			IngressNamespace: pulumi.String("networking"),
			IngressLabels: pulumi.ToStringMap(map[string]string{
				"app": "traefik",
			}),
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl -v https://%s", cm.URLs.MapIndex(pulumi.String("8080/TCP")))
		resp.Flag = variated.ToStringOutput()
		return nil
	})
}
