package scenario

import (
	"context"

	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/distribution/reference"
	"github.com/pkg/errors"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

func DecodeOCI(ctx context.Context, id, ref string, add map[string]string) (string, error) {
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
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
	}

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
