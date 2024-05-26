package common

import (
	"context"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/zap"
)

func LockTOTW(ctx context.Context) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, "totw")
}

func LockChallenge(ctx context.Context, challengeId string) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, filepath.Join("chall", fs.Hash(challengeId)))
}

func LockInstance(ctx context.Context, challengeId, sourceId string) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, filepath.Join("chall", fs.Hash(challengeId), "src", fs.Hash(sourceId)))
}

// LClose is a helper that logs any error through the lock close call.
func LClose(lock lock.RWLock) {
	logger := global.Log()
	if err := lock.Close(); err != nil {
		logger.Error(context.Background(), "lock close",
			zap.Error(err),
			zap.String("key", lock.Key()),
		)
	}
}
