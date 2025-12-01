package scenario

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/pkg/services/oci"
	"github.com/distribution/reference"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
)

// EncodeOCI is a helper function that packs a directory as a scenario,
// and distribute it as an OCI blob as the given reference.
//
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
	repo.Client, err = oci.NewORASClient(ref, username, password)
	if err != nil {
		return err
	}

	// 4. Copy from the file store to the remote repository
	_, err = oras.Copy(ctx, fs, tag, repo, tag, oras.DefaultCopyOptions)
	return err
}
