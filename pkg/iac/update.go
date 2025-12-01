package iac

import (
	"context"
	"fmt"
	"os"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"go.uber.org/zap"
)

// Update a challenge instance given an update strategy.
// You need to give the previous scenario it should be updated from in order for
// the update strategy to properly resolve the resources' delta.
func Update(
	ctx context.Context,
	previousScenario string,
	updateStrategy string,
	fschall *fs.Challenge,
	fsist *fs.Instance,
) error {
	switch updateStrategy {
	// default value such that pool claim is possible (elseway cyclic imports)
	case "update_in_place", "":
		return updateInPlace(ctx, previousScenario, fschall, fsist)

	case "blue_green":
		return blueGreen(ctx, previousScenario, fschall, fsist)

	case "recreate":
		return recreate(ctx, previousScenario, fschall, fsist)
	}
	panic(fmt.Errorf("unhandled update strategy, please open an issue: %s", updateStrategy))
}

// Update-In-Place strategy loads the existing stack and state then moves to the
// new stack and update the state.
func updateInPlace(ctx context.Context, previousScenario string, fschall *fs.Challenge, fsist *fs.Instance) error {
	return up(ctx, previousScenario, fsist.Identity, fschall, fsist)
}

// Blue Green deployment spins up a new instance and once it's done destroys the existing one.
func blueGreen(ctx context.Context, previousScenario string, fschall *fs.Challenge, fsist *fs.Instance) error {
	oldID := fsist.Identity
	fsist.Identity = identity.New()

	if err := up(ctx, previousScenario, fsist.Identity, fschall, fsist); err != nil {
		return err
	}
	return down(ctx, previousScenario, oldID, fschall, fsist)
}

// Recreate destroys the existing instance then spins up a new one.
func recreate(ctx context.Context, previousScenario string, fschall *fs.Challenge, fsist *fs.Instance) error {
	if err := down(ctx, previousScenario, fsist.Identity, fschall, fsist); err != nil {
		return err
	}
	return up(ctx, fschall.Scenario, fsist.Identity, fschall, fsist)
}

func up(ctx context.Context, scenario, id string, fschall *fs.Challenge, fsist *fs.Instance) error {
	global.Log().Info(ctx, "spinning up or updating instance", zap.String("instance", id))

	// Then load the corresponding stack
	stack, err := LoadStack(ctx, scenario, id)
	if err != nil {
		return err
	}
	if err := Additional(ctx, stack, fschall.Additional, fsist.Additional); err != nil {
		return err
	}
	if err := stack.pas.SetConfig(ctx, "identity", auto.ConfigValue{Value: id}); err != nil {
		return err
	}

	// Make sure to extract the state whatever happen, or at least try and store
	// it in the FS Instance.
	sr, err := stack.Up(ctx)
	if nerr := stack.Export(ctx, sr, fsist); nerr != nil {
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

func down(ctx context.Context, scenario, id string, fschall *fs.Challenge, fsist *fs.Instance) error {
	global.Log().Info(ctx, "destroying instance", zap.String("instance", id))

	// Then load the corresponding stack
	stack, err := LoadStack(ctx, scenario, id)
	if err != nil {
		return err
	}
	if err := Additional(ctx, stack, fschall.Additional, fsist.Additional); err != nil {
		return err
	}
	if err := stack.pas.SetConfig(ctx, "identity", auto.ConfigValue{Value: id}); err != nil {
		return err
	}

	// Make sure to extract the state whatever happen, or at least try and store
	// it in the FS Instance.
	if err := stack.Down(ctx); err != nil {
		if err := fsist.Delete(); err != nil {
			return errors.Wrap(err, "instance failed to delete, inconsistencies may occur")
		}
		return err
	}
	if err := os.RemoveAll(fs.InstanceDirectory(fschall.ID, id)); err != nil {
		return err
	}
	return nil
}
