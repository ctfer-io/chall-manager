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
) (dir string, err error) {
	// Lock this ref so only one call works on it in parallel
	// -> avoid duplicated OCI calls, and inconsistent filesystem operations
	l, _ := mg.locks.LoadOrStore(ref, &sync.Mutex{})
	lock := l.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// Check if already loaded in cache
	name, dig, err := mg.resolve(ctx, ref)
	if err != nil {
		return "", err
	}

	// Get the corresponding directory
	dir = mg.digestDirectory(dig)
	_, err = os.Stat(dir)
	if err == nil {
		return dir, nil // reference has already been pulled and validated, skip it
	}
	if !os.IsNotExist(err) { // -> an error which is not "not found" -> there is a problem
		return "", err
	}

	// Then create it
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return "", err
	}
	defer func() {
		// If there is an error, remove the directory such that a next call might fix it (e.g., a copy error, permissions issue).
		//
		// Don't catch the error to avoid wrapping meaningfull errors + should not fail so do it as a best effort.
		// Worst case, if it happens to fail, is that recovery can be performed manually.
		//
		//   rm -rf "${HOME}/.cache/chall-manager/$(crane digest <ref>)
		//
		// or automatically by restarting the app (with a fresh cache directory, of course, which happens with our IaC).
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()

	// Download the corresponding OCI artifact
	if err := mg.downloadOCI(ctx, ref, name, dig, dir); err != nil {
		return "", err
	}

	// Validate it, i.e., contains the obvious files.
	if err := mg.validate(dir); err != nil {
		switch err := err.(type) { // propagate reference as validation is performed against a directory
		case *errs.Scenario:
			err.Ref = ref
		case *errs.Preprocess:
			err.Ref = ref
		}
		return "", err
	}

	return dir, nil
}

func (mg *Manager) resolve(
	ctx context.Context,
	ref string,
) (name, dig string, err error) {
	if v, ok := mg.digCache.Load(ref); ok {
		hit := v.(*cacheEntry)
		return hit.name, hit.dig, nil
	}

	name, dig, err = resolveDigest(ctx, ref, mg.insecure, mg.username, mg.password)
	if err != nil {
		return
	}

	mg.digCache.Store(ref, &cacheEntry{
		name: name,
		dig:  dig,
	})
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
	repo.Client, err = NewORASClient(ref, mg.username, mg.password)
	if err != nil {
		return err
	}
	repo.PlainHTTP = mg.insecure

	// Copy from the remote repository to the file store
	_, err = oras.Copy(ctx,
		repo, dig, // remote image
		fs, dig, // filesystem image
		oras.DefaultCopyOptions,
	)
	if err, ok := err.(*oras.CopyError); ok {
		return &errs.OCIInteraction{
			Ref: ref,
			Sub: errors.Wrap(err, "local cache might not be properly created"),
		}
	}
	return err
}

// validate the obvious content of a Pulumi program, i.e. there exist a Pulumi.yaml/Pulumi.yml
// file that defines a Project with Go runtime, check if pre-compiled binary exists or compile
// the source code.
// If no error is returned, means the local copy of a scenario is at least runnable.
func (mg *Manager) validate(dir string) error {
	// Load the Pulumi project
	pb, fname, err := loadPulumiProject(dir)
	if err != nil {
		return &errs.Scenario{
			Sub: err,
		}
	}
	var proj workspace.Project
	if err := yaml.Unmarshal(pb, &proj); err != nil {
		return &errs.Scenario{
			Sub: errors.Wrap(err, "invalid Pulumi yaml content"),
		}
	}

	// Pre-process each runtime
	switch proj.Runtime.Name() {
	case "go":
		// If not built already, build it
		if bin, ok := proj.Runtime.Options()["binary"]; ok {
			binStr, ok := bin.(string)
			if !ok {
				return &errs.Scenario{
					Sub: errors.New("runtime.options.binary should be a string"),
				}
			}

			// Ensure it has been copied in the OCI artifact
			if _, err := os.Stat(filepath.Join(dir, binStr)); err != nil {
				if os.IsNotExist(err) {
					return &errs.Scenario{
						Sub: errors.New("runtime.options.binary is not shipped in the scenario"),
					}
				}
				return err
			}
		} else {
			// Compile it such that cache is directly usable
			if err := compile(dir); err != nil {
				return &errs.Preprocess{
					Dir: dir,
					Sub: err,
				}
			}
			proj.Runtime.SetOption("binary", "./main")

			pb, err = yaml.Marshal(proj)
			if err != nil {
				return &errs.Preprocess{
					Dir: dir,
					Sub: err,
				}
			}
			if err := os.WriteFile(filepath.Join(dir, fname), pb, 0600); err != nil {
				return &errs.Preprocess{
					Dir: dir,
					Sub: err,
				}
			}
		}

		// Make it executable (OCI does not natively copy permissions)
		if err := os.Chmod(filepath.Join(dir, proj.Runtime.Options()["binary"].(string)), 0766); err != nil {
			return &errs.Preprocess{
				Dir: dir,
				Sub: err,
			}
		}

	default:
		return &errs.Scenario{
			Sub: fmt.Errorf("unsupported runtime: %s", proj.Runtime.Name()),
		}
	}

	return nil
}

func loadPulumiProject(dir string) ([]byte, string, error) {
	var lastErr error
	for _, alt := range []string{"Pulumi.yaml", "Pulumi.yml"} {
		b, err := os.ReadFile(filepath.Join(dir, alt))
		if err == nil {
			return b, alt, nil
		}
		lastErr = err
	}
	if !os.IsNotExist(lastErr) {
		return nil, "", lastErr
	}
	return nil, "", errors.New("no Pulumi project file found")
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
