package common

import (
	"context"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func LockTOTW(ctx context.Context) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, "totw")
}

func LockChallenge(ctx context.Context, challengeID string) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, filepath.Join("chall", fs.Hash(challengeID)))
}

func LockInstance(ctx context.Context, challengeID, identity string) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, filepath.Join("chall", fs.Hash(challengeID), "src", fs.Hash(identity)))
}
