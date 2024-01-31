package sdk

import (
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Run is a SDK helper to ease the creation of challenge factories.
// You only need to provide a factory function, and the chall-manager will
// build an API around it such that it could pilot it.
func Run(f Factory) {
	project := os.Getenv("CM_PROJECT")

	pulumi.Run(func(ctx *pulumi.Context) error {
		req := &Request{
			Ctx:    ctx,
			Config: Load(ctx, project),
		}
		resp := &Response{}

		if err := f(req, resp); err != nil {
			return err
		}

		ctx.Export("connection_info", resp.ConnectionInfo)
		ctx.Export("flag", resp.Flag)
		return nil
	})
}
