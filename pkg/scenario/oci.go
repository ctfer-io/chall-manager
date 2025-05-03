package scenario

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/distribution/reference"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

func newClient(ref string, username, password *string) *auth.Client {
	cli := &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
	}
	if username != nil && password != nil {
		cli.Credential = auth.StaticCredential(ref, auth.Credential{
			Username: *username,
			Password: *password,
		})
	}
	return cli
}

// EncodeOCI is a helper function that packs a directory as a scenario,
// and distribute it as an OCI blob as the given reference.
// It is the opposite of [DecodeOCI].
func EncodeOCI(ctx context.Context, ref, dir string, username, password *string) error {
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
	repo.PlainHTTP = true
	repo.Client = newClient(ref, username, password)

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
	username, password *string,
) (string, error) {
	rr, err := reference.Parse(ref)
	if err != nil {
		return "", err
	}
	r, ok := rr.(reference.NamedTagged)
	if !ok {
		return "", errors.New("invalid reference format, does not implement reference.NamedTagged")
	}

	// 0. Create a file store
	dir, err := fs.RefDirectory(id, ref)
	if err != nil {
		return "", err
	}
	fs, err := file.New(dir)
	if err != nil {
		return "", err
	}
	defer fs.Close()

	// 1. Connect to a remote repository
	repo, err := remote.NewRepository(r.Name())
	if err != nil {
		return "", err
	}
	repo.PlainHTTP = true
	// Note: The below code can be omitted if authentication is not required
	repo.Client = newClient(ref, username, password)

	// 2. Copy from the remote repository to the file store
	if _, err := oras.Copy(ctx,
		repo, r.Tag(), // remote image
		fs, r.Tag(), // filesystem image
		oras.DefaultCopyOptions,
	); err != nil {
		return "", err
	}

	// 3. Validate
	return dir, Validate(ctx, dir, add)
}
