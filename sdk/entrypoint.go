package sdk

import (
	"os"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
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
		resp := &Response{}

		opts := []pulumi.ResourceOption{}

		// Leveraging the OCI configuration of the chall-manager, a ChallOps
		// can build Docker images on the fly specific to an identity.
		oci_registry_url, ok_oci_registry_url := os.LookupEnv("OCI_REGISTRY_URL")
		oci_registry_username, ok_oci_registry_username := os.LookupEnv("OCI_REGISTRY_USERNAME")
		oci_registry_password, ok_oci_registry_password := os.LookupEnv("OCI_REGISTRY_PASSWORD")
		if ok_oci_registry_url {
			args := &docker.ProviderRegistryAuthArgs{
				Address: pulumi.String(oci_registry_url),
			}
			if ok_oci_registry_username {
				args.Username = pulumi.String(oci_registry_username)
			}
			if ok_oci_registry_password {
				args.Password = pulumi.String(oci_registry_password)
			}

			pv, err := docker.NewProvider(ctx, "docker", &docker.ProviderArgs{
				RegistryAuth: docker.ProviderRegistryAuthArray{
					args,
				},
			})
			if err != nil {
				return errors.Wrap(err, "creating pre-configured docker provider")
			}

			opts = append(opts, pulumi.Provider(pv))
		}

		if err := f(req, resp, opts...); err != nil {
			return err
		}

		ctx.Export("connection_info", resp.ConnectionInfo)
		ctx.Export("flag", resp.Flag)
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
	Flag           pulumi.StringOutput
}

// Configuration is the struct that contains the flattened configuration
// from a chall-manager stack up.
type Configuration struct {
	Identity string
}

// Load flatten the Pulumi stack configuration into a ready-to-use struct.
func Load(ctx *pulumi.Context, project string) *Configuration {
	cfg := config.New(ctx, project)

	return &Configuration{
		Identity: cfg.Get("identity"),
	}
}
