package lock

import (
	"context"

	"github.com/ctfer-io/chall-manager/global"
)

// RWLock define an implementation of a readers-writer lock with writer-preference.
//
// Locks should be short-lived and recover from previous states without the need
// to persist them in memory (for fault-tolerancy and scalability).
// This imply the context should be passed to the constructor rather than methods.
type RWLock interface {
	Key() string

	// RLock is a reader lock
	RLock() error
	// RUnlock is a reader unlock
	RUnlock() error

	// RWLock is a writer lock, thus as priority over readers
	RWLock() error
	// RWUnlock is a writer unlock
	RWUnlock() error

	// Close network socket/connections
	Close() error
}

func NewRWLock(ctx context.Context, key string) (RWLock, error) {
	switch global.Conf.Lock.Kind {
	case "local":
		return NewLocalRWLock(ctx, key)
	case "etcd":
		return NewEtcdRWLock(ctx, key)
	}
	panic("unhandled lock kind " + global.Conf.Lock.Kind)
}
