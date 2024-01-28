package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blend/go-sdk/filelock"
	"github.com/ctfer-io/chall-manager/global"
)

func flock(_ context.Context, id string) (func() error, error) {
	// Open (or create if necessary) the identity lock file
	lname := filepath.Join(global.Conf.Directory, "states", id) + ".lock"
	f, err := os.Open(lname)
	if err != nil {
		f, err = os.Create(lname)
		if err != nil {
			return nil, fmt.Errorf("cannot create lock file for %s", id)
		}
	}

	// Create flock
	if err := filelock.Lock(f); err != nil {
		return nil, fmt.Errorf("failed to acquire flock for %s", id)
	}

	// Build release function
	return func() error {
		if err := filelock.Unlock(f); err != nil {
			return fmt.Errorf("failed to release flock for %s", id)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("cannot close lock file for %s", id)
		}
		return nil
	}, nil
}
