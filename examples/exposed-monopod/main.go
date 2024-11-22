package main

import (
	"github.com/ctfer-io/chall-manager/sdk"
	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		cm, err := k8s.NewExposedMonopod(req.Ctx, &k8s.ExposedMonopodArgs{
			Image:      pulumi.String("pandatix/license-lvl1:latest"),
			Port:       pulumi.Int(8080),
			ExposeType: k8s.ExposeIngress,
			Hostname:   pulumi.String("brefctf.ctfer.io"),
			Identity:   pulumi.String(req.Config.Identity),
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl -v https://%s", cm.URL)
		return nil
	})
}
