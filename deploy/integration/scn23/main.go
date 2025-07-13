package main

import (
	"github.com/ctfer-io/chall-manager/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	year = "2023"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		resp.ConnectionInfo = pulumi.Sprintf("https://ctfer.io/blog/24h-iut-%s", year)
		return nil
	})
}
