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
//
// Context errors (i.e. canceled or deadline reached) are not returned, only errors
// from the downstream service.
// Correct use of this module is to check yourself ctx.Err() whenever needing to.
type RWLock interface {
	Key() string

	// RLock is a reader lock
	RLock(context.Context) error
	// RUnlock is a reader unlock
	RUnlock(context.Context) error

	// RWLock is a writer lock, thus as priority over readers
	RWLock(context.Context) error
	// RWUnlock is a writer unlock
	RWUnlock(context.Context) error

	// Close sessions
	Close() error
}

func NewRWLock(ctx context.Context, key string) (RWLock, error) {
	if global.Conf.Etcd.Endpoint == "" {
		return NewLocalRWLock(key)
	}
	return NewEtcdRWLock(ctx, key)
}
