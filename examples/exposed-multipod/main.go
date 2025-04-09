package main

import (
	"github.com/ctfer-io/chall-manager/sdk"
	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// This example comes from NoBrackets 2024 Final challenges.
// Source: https://github.com/nobrackets-ctf/INFRA-NoBrackets-CTF-2024-Challenges-Finale/tree/main/Web/vip-only
//
// Challenge by @ribt
// Original Chall-Manager scenario by @pandatix

const (
	baseFlag = "Only_VIPs_are_allowed_here!"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		// Create the specific resources we want, ouside of the ExposedMultipod
		flag := pulumi.Sprintf("NBCTF{%s}", sdk.Variate(req.Config.Identity, baseFlag))

		// Then deploy our ExposedMultipod, and provide it our specific content
		emp, err := k8s.NewExposedMultipod(req.Ctx, "vip-only", &k8s.ExposedMultipodArgs{
			Identity: pulumi.String(req.Config.Identity),
			Hostname: pulumi.String("brefctf.ctfer.io"),
			Containers: k8s.ContainerMap{
				"node": k8s.ContainerArgs{
					Image: pulumi.String("pandatix/vip-only-node:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(3000),
							ExposeType: k8s.ExposeIngress,
						},
					},
					Envs: k8s.PrinterMap{
						"SESSION_SECRET": k8s.NewPrinter("0A010010D98FDFDJDJHIUAY"),
						"MONGO_URI":      k8s.NewPrinter("mongodb://root:5e409bd6c906e75bc961de62d516ca52@%s:27017/vipOnlyApp?authSource=admin", "mongo"),
					},
					Files: pulumi.StringMap{
						"/app/flag.txt": flag,
					},
				},
				"mongo": k8s.ContainerArgs{
					Image: pulumi.String("pandatix/vip-only-mongo:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port: pulumi.Int(27017),
						},
					},
					Envs: k8s.PrinterMap{
						"MONGO_INITDB_DATABASE":      k8s.ToPrinter(pulumi.String("vipOnlyApp")),
						"MONGO_INITDB_ROOT_USERNAME": k8s.ToPrinter(pulumi.String("root")),
						"MONGO_INITDB_ROOT_PASSWORD": k8s.ToPrinter(pulumi.String("5e409bd6c906e75bc961de62d516ca52")),
					},
				},
			},
			Rules: k8s.RuleArray{
				k8s.RuleArgs{
					From: pulumi.String("node"),
					To:   pulumi.String("mongo"),
					On:   pulumi.Int(27017),
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

		resp.ConnectionInfo = pulumi.Sprintf("curl -v https://%s", emp.URLs.MapIndex(pulumi.String("node")).MapIndex(pulumi.String("3000/TCP")))
		resp.Flag = flag
		return nil
	})
}
