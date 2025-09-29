package scenario

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
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

// EncodeOCI is a helper function that packs a directory as a scenario,
// and distribute it as an OCI blob as the given reference.
// It is the opposite of [DecodeOCI].
func EncodeOCI(ctx context.Context, ref, dir string, insecure bool, username, password string) error {
	// Create a file store
	fs, err := file.New(dir)
	if err != nil {
		return err
	}
	defer fs.Close()

	// Add files to the file store
	fileDescriptors := []v1.Descriptor{}
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		fileDescriptor, err := fs.Add(ctx, rel, "application/vnd.ctfer-io.file", "")
		if err != nil {
			return err
		}
		fileDescriptors = append(fileDescriptors, fileDescriptor)

		return nil
	}); err != nil {
		return err
	}

	// Pack the files and tag the packed manifest
	manifestDescriptor, err := oras.PackManifest(ctx, fs,
		oras.PackManifestVersion1_1,
		"application/vnd.ctfer-io.scenario",
		oras.PackManifestOptions{Layers: fileDescriptors},
	)
	if err != nil {
		return err
	}

	rr, err := reference.Parse(ref)
	if err != nil {
		return err
	}
	rt, ok := rr.(reference.Tagged)
	if !ok {
		return errors.New("invalid reference format, may miss a tag")
	}

	tag := rt.Tag()
	if err = fs.Tag(ctx, manifestDescriptor, tag); err != nil {
		return err
	}

	// 3. Connect to a remote repository
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return err
	}
	if insecure {
		repo.PlainHTTP = true
	}
	repo.Client, err = NewORASClient(ref, username, password)
	if err != nil {
		return err
	}

	// 4. Copy from the file store to the remote repository
	_, err = oras.Copy(ctx, fs, tag, repo, tag, oras.DefaultCopyOptions)
	return err
}

// DecodeOCI is a helper function that unpacks a given reference to an OCI blob
// of data containing a scenario, and puts it in the directory of a given challenge
// by its id.
// It is the opposite of [EncodeOCI].
func DecodeOCI(
	ctx context.Context,
	id, ref string,
	add map[string]string,
	insecure bool,
	username, password string,
) (string, error) {
	// Resolve the digest if none is defined
	name, dig, err := resolve(ctx, ref, insecure, username, password)
	if err != nil {
		return "", err
	}

	// Get the corresponding directory
	dir := filepath.Join(global.CacheDir(), "oci", dig)
	fs, err := file.New(dir)
	if err != nil {
		return "", err
	}
	defer fs.Close()

	// 1. Connect to a remote repository
	repo, err := remote.NewRepository(name)
	if err != nil {
		return "", err
	}
	if insecure {
		repo.PlainHTTP = true
	}
	repo.Client, err = NewORASClient(ref, username, password)
	if err != nil {
		return "", err
	}

	// 2. Copy from the remote repository to the file store
	if _, err := oras.Copy(ctx,
		repo, dig, // remote image
		fs, dig, // filesystem image
		oras.DefaultCopyOptions,
	); err != nil {
		return "", err
	}

	// 3. Validate
	return dir, Validate(ctx, dir, add)
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
