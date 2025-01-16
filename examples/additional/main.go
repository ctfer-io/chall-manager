package main

import (
	"encoding/json"

	"github.com/ctfer-io/chall-manager/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		b, err := json.Marshal(req.Config.Additional)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.String(string(b)).ToStringOutput()
		return nil
	})
}
