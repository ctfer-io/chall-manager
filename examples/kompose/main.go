package main

import (
	_ "embed"

	"github.com/ctfer-io/chall-manager/sdk"
	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// This example comes from NoBrackets 2024 Final challenges.
// Source: https://github.com/nobrackets-ctf/INFRA-NoBrackets-CTF-2024-Challenges-Finale/tree/main/Web/vip-only
//
// Challenge by @ribt
// Original Chall-Manager scenario by @pandatix

//go:embed docker-compose.yaml
var dc string

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		kmp, err := k8s.NewKompose(req.Ctx, "vip-only", &k8s.KomposeArgs{
			Identity: pulumi.String(req.Config.Identity),
			Hostname: pulumi.String("24hiut25.ctfer.io"),
			YAML:     pulumi.String(dc),
			Ports: k8s.PortBindingMapArray{
				"node": {
					k8s.PortBindingArgs{
						Port:       pulumi.Int(3000),
						ExposeType: k8s.ExposeIngress,
					},
				},
			},
			// The following fits for a Nginx-based use case
			IngressAnnotations: pulumi.StringMap{
				"kubernetes.io/ingress.class":                  pulumi.String("nginx"),
				"nginx.ingress.kubernetes.io/backend-protocol": pulumi.String("HTTP"),
				"nginx.ingress.kubernetes.io/ssl-redirect":     pulumi.String("true"),
				"nginx.ingress.kubernetes.io/proxy-body-size":  pulumi.String("50m"),
			},
			IngressNamespace: pulumi.String("ingress-nginx"),
			IngressLabels: pulumi.ToStringMap(map[string]string{
				"app.kubernetes.io/component": "controller",
				"app.kubernetes.io/instance":  "ingress-nginx",
			}),
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl http://%s", kmp.URLs.MapIndex(pulumi.String("node")).MapIndex(pulumi.String("3000/TCP")))
		return nil
	})
}
