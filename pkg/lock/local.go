package lock

import (
	"context"
	"sync"
)

var (
	localLocks sync.Map
)

type LocalLock struct {
	key string
	mx  *sync.RWMutex
}

var _ RWLock = (*LocalLock)(nil)

func NewLocalRWLock(key string) (RWLock, error) {
	lock, _ := localLocks.LoadOrStore(key, &LocalLock{
		key: key,
		mx:  &sync.RWMutex{},
	})
	return lock.(RWLock), nil
}

func (lock *LocalLock) Key() string {
	return lock.key
}

func (lock *LocalLock) IsCanceled(err error) bool {
	return err == context.Canceled
}

func (lock *LocalLock) RLock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lock.mx.RLock()
	return nil
}

func (lock *LocalLock) RUnlock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lock.mx.RUnlock()
	return nil
}

func (lock *LocalLock) RWLock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lock.mx.Lock()
	return nil
}

func (lock *LocalLock) RWUnlock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lock.mx.Unlock()
	return nil
}
