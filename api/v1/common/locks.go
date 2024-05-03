package common

import (
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/zap"
)

func LockTOTW() (lock.RWLock, error) {
	return lock.NewRWLock("totw")
}

func LockChallenge(challengeId string) (lock.RWLock, error) {
	return lock.NewRWLock("chall/" + challengeId)
}

func LockInstance(challengeId, sourceId string) (lock.RWLock, error) {
	return lock.NewRWLock("chall/" + challengeId + "/src/" + sourceId)
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
