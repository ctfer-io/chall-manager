package lock

import (
	"context"
	"sync"
)

type LocalLock struct {
	key string
	mx  *sync.RWMutex
}

func NewLocalRWLock(_ context.Context, key string) (RWLock, error) {
	return &LocalLock{
		key: key,
		mx:  &sync.RWMutex{},
	}, nil
}

func (lock *LocalLock) Key() string {
	return lock.key
}

func (lock *LocalLock) RLock() error {
	lock.mx.RLock()
	return nil
}

func (lock *LocalLock) RUnlock() error {
	lock.mx.RUnlock()
	return nil
}

func (lock *LocalLock) RWLock() error {
	lock.mx.Lock()
	return nil
}

func (lock *LocalLock) RWUnlock() error {
	lock.mx.Unlock()
	return nil
}

func (lock *LocalLock) Close() error {
	return nil
}
