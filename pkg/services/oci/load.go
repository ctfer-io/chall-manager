package oci

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"go.uber.org/multierr"
	"go.yaml.in/yaml/v2"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

type cacheEntry struct {
	name string
	dig  string
}

// Load a reference (a deployment scenario, distributed as an OCI artifact).
// It will also ensure its basic validity, i.e., if it is a supported runtime,
// and if uses a binary that it has been copied.
//
// Returns the directory it has been loaded into, ready to use, or an error.
func (mg *Manager) Load(
	ctx context.Context,
	ref string,
) (string, error) {
	// Lock this ref so only one call works on it in parallel
	// -> avoid duplicated OCI calls, and inconsistent filesystem operations
	l, _ := mg.locks.LoadOrStore(ref, &sync.Mutex{})
	lock := l.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// Check if already loaded in cache
	name, dig, err := mg.resolve(ctx, ref, mg.insecure, mg.username, mg.password)
	if err != nil {
		return "", err
	}

	// Get the corresponding directory
	dir := mg.digestDirectory(dig)
	_, err = os.Stat(dir)
	if err == nil {
		return dir, nil
	}
	if !os.IsNotExist(err) { // -> an error which is not "not found" -> there is a problem
		return "", &errs.ErrInternal{
			Sub: err,
		}
	}

	// Download the corresponding OCI artifact
	if err := mg.downloadOCI(ctx, ref, name, dig, dir); err != nil {
		return "", err
	}

	// Validate it.
	// If there is an error, remove the directory such that a next call might
	// fix it (e.g., if a transient error).
	if err := mg.validate(dir); err != nil {
		return "", multierr.Append(err, os.RemoveAll(dir))
	}

	return dir, nil
}

func (mg *Manager) resolve(
	ctx context.Context,
	ref string,
	insecure bool,
	username, password string,
) (name, dig string, err error) {
	if hit, ok := mg.digCache[ref]; ok {
		return hit.name, hit.dig, nil
	}

	name, dig, err = resolve(ctx, ref, insecure, username, password)
	if err != nil {
		return
	}

	mg.digCache[ref] = &cacheEntry{
		name: name,
		dig:  dig,
	}
	return
}

func (mg *Manager) digestDirectory(dig string) string {
	return filepath.Join(mg.cacheDir(), "oci", dig)
}

func (mg *Manager) downloadOCI(
	ctx context.Context,
	ref, name, dig, dir string,
) error {
	// Create the OCI filesystem
	fs, err := file.New(dir)
	if err != nil {
		return err
	}
	defer fs.Close()

	// Connect to a remote repository
	repo, err := remote.NewRepository(name)
	if err != nil {
		return err
	}
	if mg.insecure {
		repo.PlainHTTP = true
	}
	repo.Client, err = NewORASClient(ref, mg.username, mg.password)
	if err != nil {
		return err
	}

	// Copy from the remote repository to the file store
	_, err = oras.Copy(ctx,
		repo, dig, // remote image
		fs, dig, // filesystem image
		oras.DefaultCopyOptions,
	)
	return err
}

func (mg *Manager) validate(dir string) error {
	// Get project name
	b, fname, err := loadPulumiYml(dir)
	if err != nil {
		return &errs.ErrInternal{Sub: errors.Wrap(err, "invalid scenario")}
	}
	var yml workspace.Project
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return &errs.ErrInternal{Sub: errors.Wrap(err, "invalid Pulumi yaml content")}
	}

	switch yml.Runtime.Name() {
	case "go":
		// If not built already, build it
		if bin, ok := yml.Runtime.Options()["binary"]; ok {
			binStr, ok := bin.(string)
			if !ok {
				return &errs.ErrScenario{
					Sub: errors.New("runtime options binary should be a string"),
				}
			}

			// Ensure it has been copied in the OCI artifact
			if _, err := os.Stat(filepath.Join(dir, binStr)); err != nil {
				if os.IsNotExist(err) {
					return &errs.ErrScenario{
						Sub: errors.New("runtime options binary is not contained in the scenario"),
					}
				}
				return &errs.ErrInternal{
					Sub: err,
				}
			}
		} else {
			// Compile it such that cache is directly usable
			if err := compile(dir); err != nil {
				return err
			}
			yml.Runtime.SetOption("binary", "./main")

			b, err = yaml.Marshal(yml)
			if err != nil {
				return &errs.ErrInternal{Sub: err}
			}
			if err := os.WriteFile(filepath.Join(dir, fname), b, 0o600); err != nil {
				return &errs.ErrInternal{Sub: err}
			}
		}

		// Make it executable (OCI does not natively copy permissions)
		if err := os.Chmod(filepath.Join(dir, yml.Runtime.Options()["binary"].(string)), 0o766); err != nil {
			return err
		}

	default:
		return fmt.Errorf("got unsupported runtime: %s", yml.Runtime.Name())
	}

	return nil
}

func loadPulumiYml(dir string) ([]byte, string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	if err == nil {
		return b, "Pulumi.yaml", nil
	}
	b, err = os.ReadFile(filepath.Join(dir, "Pulumi.yml"))
	if err == nil {
		return b, "Pulumi.yml", nil
	}
	return nil, "", err
}

func compile(dir string) error {
	mainPath := filepath.Join(dir, "main")
	cmd := exec.Command("go", "build", "-o", mainPath, mainPath+".go")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "%s", out)
	}
	return nil
}
