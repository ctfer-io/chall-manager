package common

import (
	"context"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func LockTOTW() (lock.RWLock, error) {
	return lock.NewRWLock("totw")
}

func LockChallenge(challengeID string) (lock.RWLock, error) {
	return lock.NewRWLock(filepath.Join("chall", fs.Hash(challengeID)))
}

func LockInstance(challengeID, identity string) (lock.RWLock, error) {
	return lock.NewRWLock(filepath.Join("chall", fs.Hash(challengeID), "src", fs.Hash(identity)))
}

// LClose is a helper that logs any error during the lock close call.
func LClose(lock lock.RWLock) {
	logger := global.Log()
	if err := lock.Close(); err != nil {
		logger.Error(context.Background(), "lock close",
			zap.Error(err),
			zap.String("key", lock.Key()),
		)
	}
}
