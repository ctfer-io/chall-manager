package common

import (
	"context"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/zap"
)

func LockTOTW(ctx context.Context) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, "totw")
}

func LockChallenge(ctx context.Context, challengeId string) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, "chall/"+challengeId)
}

func LockInstance(ctx context.Context, challengeId, sourceId string) (lock.RWLock, error) {
	return lock.NewRWLock(ctx, "chall/"+challengeId+"/src/"+sourceId)
}

// LClose is a helper that logs any error through the lock close call.
func LClose(lock lock.RWLock) {
	logger := global.Log()
	if err := lock.Close(); err != nil {
		logger.Error("lock close",
			zap.Error(err),
			zap.String("key", lock.Key()),
		)
	}
}
