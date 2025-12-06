package oci

import (
	"context"
	"fmt"
	"net/http"

	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
		return nil, err
	}
	r, ok := rr.(reference.Named)
	if !ok {
		return nil, errors.New("invalid reference format, does not implement reference.Named")
	}

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
func resolve(
	ctx context.Context,
	ref string,
	insecure bool,
	username, password string,
) (name string, digest string, err error) {
	// Parse the OCI reference
	r, err := reference.Parse(ref)
	if err != nil {
		return
	}

	_, canonical := r.(reference.Canonical)
	_, namedTagged := r.(reference.NamedTagged)

	// Check the digest format is valid, i.e. is sha256
	if canonical {
		alg := r.(reference.Canonical).Digest().Algorithm()
		if alg != sha256 {
			err = fmt.Errorf("unsupported algorithm, got %s but require %s", alg, sha256)
			return
		}
	}

	// If tag is not defined, default to latest
	if !namedTagged {
		tag := "latest"
		r, err = reference.Parse(fmt.Sprintf("%s:%s", ref, tag))
		if err != nil {
			return
		}
	}

	// Then get digest from upstream
	opts := []crane.Option{
		crane.WithContext(ctx),
	}
	if insecure {
		opts = append(opts, crane.Insecure)
	}
	if username != "" && password != "" {
		opts = append(opts, crane.WithAuth(&authn.Basic{
			Username: username,
			Password: password,
		}))
	}
	dig, err := crane.Digest(ref, opts...)
	if err != nil {
		return
	}

	return r.(reference.Named).Name(), dig, nil
}
