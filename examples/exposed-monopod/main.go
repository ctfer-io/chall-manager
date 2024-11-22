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

		cm, err := k8s.NewExposedMonopod(req.Ctx, &k8s.ExposedMonopodArgs{
			Image:      pulumi.String("pandatix/license-lvl1:latest"),
			Port:       pulumi.Int(8080),
			ExposeType: k8s.ExposeIngress,
			Hostname:   pulumi.String("brefctf.ctfer.io"),
			Identity:   pulumi.String(req.Config.Identity),
			Files: pulumi.StringMap{
				"/app/flag.txt": variated,
			},
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl -v https://%s", cm.URL)
		resp.Flag = variated.ToStringOutput()
		return nil
	})
}
