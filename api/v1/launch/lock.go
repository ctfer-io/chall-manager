package launch

import (
	"context"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/zap"
)

// TOTWLock return the Launch Top-Of-The-World lock, but you need to close your
// connection once you're done.
// Most often, this translates to a defer call after checking this function's
// call error.
func TOTWLock(ctx context.Context) (l lock.RWLock, err error) {
	return lock.NewRWLock("totw")
}

func lclose(lock lock.RWLock) {
	logger := global.Log()
	if err := lock.Close(); err != nil {
		logger.Error("lock close",
			zap.Error(err),
			zap.String("key", lock.Key()),
		)
	}
}
