package main

import (
	"github.com/ctfer-io/chall-manager/sdk"
	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		cm, err := k8s.NewExposedMonopod(req.Ctx, &k8s.ExposedMonopodArgs{
			Image:      "pandatix/license-lvl1:latest",
			Port:       8080,
			ExposeType: k8s.ExposeIngress,
			Hostname:   "brefctf.ctfer-io.lab",
			Identity:   req.Config.Identity,
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl -v https://%s", cm.URL)
		return nil
	})
}
