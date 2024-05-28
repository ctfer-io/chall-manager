package iac

import (
	"context"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func Update(ctx context.Context, oldDir string, fschall *fs.Challenge, fsist *fs.Instance) error {
	switch fschall.UpdateStrategy {
	case "update_in_place":
		return updateInPlace(ctx, fschall, fsist)
	case "blue_green":
		return blueGreen(ctx, oldDir, fschall, fsist)
	case "recreate":
		return recreate(ctx, oldDir, fschall, fsist)
	}
	panic("unhandled update strategy: " + fschall.UpdateStrategy)
}

// Update-In-Place strategy loads the existing stack and state then moves to the
// new stack and update the state.
func updateInPlace(ctx context.Context, fschall *fs.Challenge, fsist *fs.Instance) error {
	return up(ctx, fschall.Directory, fsist.Identity, fsist)
}

// Blue Green deployment spins up a new instance in parallel and once
// it's done destroys the existing one%
func blueGreen(ctx context.Context, oldDir string, fschall *fs.Challenge, fsist *fs.Instance) error {
	oldId := fsist.Identity
	fsist.Identity = identity.Compute(fschall.ID, fsist.SourceID)

	if err := up(ctx, fschall.Directory, fsist.Identity, fsist); err != nil {
		return err
	}
	return down(ctx, oldDir, oldId, fsist)
}

// Recreate destroys the existing instance then spins up a new one.
func recreate(ctx context.Context, oldDir string, fschall *fs.Challenge, fsist *fs.Instance) error {
	if err := down(ctx, oldDir, fsist.Identity, fsist); err != nil {
		return err
	}
	return up(ctx, fschall.Directory, fsist.Identity, fsist)
}

func up(ctx context.Context, dir, id string, fsist *fs.Instance) error {
	global.Log().Info(ctx, "spinning up or updating existing instance")

	stack, err := LoadStack(ctx, dir, id)
	if err != nil {
		return err
	}
	if err := stack.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{
			Value: id,
		},
	}); err != nil {
		return err
	}

	// Make sure to extract the state whatever happen, or at least try and store
	// it in the FS Instance.
	sr, err := stack.Up(ctx)
	if nerr := Extract(ctx, stack, sr, fsist); nerr != nil {
		if fserr := fsist.Save(); fserr != nil {
			return err
		}
		return err
	}
	if err != nil {
		if fserr := fsist.Save(); fserr != nil {
			return err
		}
		return err
	}
	return nil
}

func down(ctx context.Context, dir, id string, fsist *fs.Instance) error {
	stack, err := LoadStack(ctx, dir, id)
	if err != nil {
		return err
	}
	if err := stack.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{
			Value: id,
		},
	}); err != nil {
		return err
	}

	// Make sure to extract the state whatever happen, or at least try and store
	// it in the FS Instance.
	if _, err := stack.Destroy(ctx); err != nil {
		if err := fsist.Delete(); err != nil {
			return errors.Wrap(err, "instance failed to delete, inconsistencies may occur")
		}
		return err
	}
	return nil
}
