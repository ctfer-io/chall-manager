package sdk

import (
	"os"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

var (
	pStrEmpty = pulumi.String("").ToStringOutput()
)

// Factory define the prototype a IaC factory have to implement to be used
// by the SDK.
type Factory func(req *Request, resp *Response, opts ...pulumi.ResourceOption) error

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
		resp := &Response{
			ConnectionInfo: pStrEmpty,
			Flag:           pStrEmpty,
		}

		opts := []pulumi.ResourceOption{}

		if k8sns, ok := os.LookupEnv("KUBERNETES_TARGET_NAMESPACE"); ok {
			pv, err := kubernetes.NewProvider(ctx, "target", &kubernetes.ProviderArgs{
				Namespace: pulumi.String(k8sns),
			})
			if err != nil {
				return err
			}
			opts = append(opts, pulumi.Provider(pv))
		}

		if err := f(req, resp, opts...); err != nil {
			return err
		}

		ctx.Export("connection_info", resp.ConnectionInfo)
		if resp.Flag != pStrEmpty {
			ctx.Export("flag", resp.Flag)
		}
		ctx.Export("flags", resp.Flags)

		return nil
	})
}

// Request sent by the chall-manager SDK to the IaC factory.
type Request struct {
	Ctx    *pulumi.Context
	Config *Configuration
}

// Response is created and returned by a factory to the SDK in order to
// respond to the chall-manager API call once IaC ran.
type Response struct {
	ConnectionInfo pulumi.StringOutput

	// Deprecated: use the Flags string array output
	Flag pulumi.StringOutput

	Flags pulumi.StringArrayOutput
}

// Configuration is the struct that contains the flattened configuration
// from a chall-manager stack up.
type Configuration struct {
	Identity   string
	Additional map[string]string
}

// Load flatten the Pulumi stack configuration into a ready-to-use struct.
func Load(ctx *pulumi.Context, project string) *Configuration {
	cfg := config.New(ctx, project)

	additional := map[string]string{}
	if err := cfg.GetObject("additional", &additional); err != nil {
		panic(err)
	}
	return &Configuration{
		Identity:   cfg.Get("identity"),
		Additional: additional,
	}
}
