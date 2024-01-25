package sdk

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// Factory define the prototype a IaC factory have to implement to be used
// by the SDK.
type Factory func(req *Request, resp *Response, opts ...pulumi.ResourceOption) error

// Request sent by the chall-manager SDK to the IaC factory.
type Request struct {
	Ctx    *pulumi.Context
	Config *Configuration
}

// Response is created and returned by a factory to the SDK in order to
// respond to the chall-manager API call once IaC ran.
type Response struct {
	ConnectionInfo pulumi.StringOutput
}

// Configuration is the struct that contains the flattened configuration
// from a chall-manager stack up.
type Configuration struct {
	Identity string
	SourceID string
}

// Load flatten the Pulumi stack configuration into a ready-to-use struct.
func Load(ctx *pulumi.Context, project string) *Configuration {
	cfg := config.New(ctx, project)

	return &Configuration{
		Identity: cfg.Get("identity"),
		SourceID: cfg.Get("source_id"),
	}
}
