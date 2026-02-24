package oci

import (
	"context"
	"fmt"
	"net/http"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/distribution/reference"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	sha256 = "sha256"
)

// NewORASClient creates an ORAS client, possibly authenticated.
func NewORASClient(ref string, username, password string) (*auth.Client, error) {
	// Parse reference
	rr, err := reference.Parse(ref)
	if err != nil {
		return nil, &errs.MalformedOCIReference{
			Ref: ref,
			Sub: err,
		}
	}
	r := rr.(reference.Named)

	// Build client
	cli := &auth.Client{
		Client: &http.Client{
			Transport: otelhttp.NewTransport(retry.NewTransport(nil)),
		},
		Cache: auth.NewCache(),
	}
	if username != "" && password != "" {
		cli.Credential = auth.StaticCredential(reference.Domain(r), auth.Credential{
			Username: username,
			Password: password,
		})
	}

	return cli, nil
}

// Resolves a reference toward its registry.
// Returns the name of the image and its digest (along the algorithm), or an error.
func resolveDigest(
	ctx context.Context,
	ref string,
	insecure bool,
	username, password string,
) (name string, digest string, err error) {
	// Parse the OCI reference
	pr, err := registry.ParseReference(ref)
	if err != nil {
		return "", "", &errs.MalformedOCIReference{
			Ref: ref,
			Sub: err,
		}
	}

	// If tag is not defined, default to latest
	if pr.Reference == "" {
		pr.Reference = "latest"
		ref = pr.String()
	}

	// Look if there is a digest defined
	if dig, err := pr.Digest(); err == nil {
		if dig.Algorithm().String() != sha256 {
			return "", "", &errs.MalformedOCIReference{
				Ref: ref,
				Sub: fmt.Errorf("unsupported algorithm, got %s but require %s", dig.Algorithm(), sha256),
			}
		}
	}

	// Create client to interact with the registry
	cli, err := NewORASClient(ref, username, password)
	if err != nil {
		return "", "", err // XXX not managed ?
	}
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return "", "", &errs.MalformedOCIReference{
			Ref: ref,
			Sub: err,
		}
	}
	repo.PlainHTTP = insecure
	repo.Client = cli

	// Resolve the reference descriptor
	descriptor, err := repo.Resolve(ctx, ref)
	if err != nil {
		return "", "", &errs.OCIInteraction{
			Ref: ref,
			Sub: err,
		}
	}

	// Check it is gives the expected digest algorithm
	if descriptor.Digest.Algorithm().String() != sha256 {
		return "", "", &errs.OCIInteraction{
			Ref: ref,
			Sub: fmt.Errorf(
				"registry %s returned a digest in %s, expected %s",
				pr.Registry,
				descriptor.Digest.Algorithm().String(),
				sha256,
			),
		}
	}

	return fmt.Sprintf("%s/%s", pr.Registry, pr.Repository), descriptor.Digest.String(), nil
}
