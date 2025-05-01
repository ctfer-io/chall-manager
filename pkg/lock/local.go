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

func (lock *LocalLock) RLock(_ context.Context) error {
	lock.mx.RLock()
	return nil
}

func (lock *LocalLock) RUnlock(_ context.Context) error {
	lock.mx.RUnlock()
	return nil
}

func (lock *LocalLock) RWLock(_ context.Context) error {
	lock.mx.Lock()
	return nil
}

func (lock *LocalLock) RWUnlock(_ context.Context) error {
	lock.mx.Unlock()
	return nil
}

func (lock *LocalLock) Close() error {
	return nil
}
