package lock

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ctfer-io/chall-manager/global"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

// The etcd distributed lock enables you to have a powerful mutual exclusion (mutex)
// system in with an etcd cluster.
// You can't use a file lock in distributed environments such as Kubernetes because
// the Pods are isolated in there own contexts hence would not share the filelock
// information.
//
// This implementation goes further than a simple mutex, as it implements the
// readers-writer lock for a writer-preference.
//
// It assumes the network is reliable.
// Moreover, it is unfair as it does not use a queue to order requests as a FIFO.
//
// Based upon 'Concurrent Control with "Readers" and "Writers"' by Courtois et al. (1971)
// DOI: 10.1145/362759.362813
type EtcdRWLock struct {
	key string
	s   *concurrency.Session

	// readCounter  -> /chall-manager/<key>/readCounter
	// writeCounter -> /chall-manager/<key>/writeCounter
	m1, m2, m3, r, w *concurrency.Mutex
	// m1 -> /chall-manager/<key>/m1
	// m2 -> /chall-manager/<key>/m2
	// m3 "prevents too many readers from waiting on mutex r, so writers have a good
	// chance to signal r when they come", from user "Attala" on Stackoverflow.
	// Ref: https://stackoverflow.com/questions/9974384/second-algorithm-solution-to-readers-writer
	// m3 -> /chall-manager/<key>/m3
	// r  -> /chall-manager/<key>/r
	// w  -> /chall-manager/<key>/w
}

func NewEtcdRWLock(ctx context.Context, key string) (RWLock, error) {
	s, err := global.GetEtcdManager().NewConcurrencySession(ctx)
	if err != nil {
		return nil, err
	}

	pfx := "/chall-manager/" + key + "/"
	return &EtcdRWLock{
		key: key,
		s:   s,
		m1:  concurrency.NewMutex(s, pfx+"m1"),
		m2:  concurrency.NewMutex(s, pfx+"m2"),
		m3:  concurrency.NewMutex(s, pfx+"m3"),
		r:   concurrency.NewMutex(s, pfx+"r"),
		w:   concurrency.NewMutex(s, pfx+"w"),
	}, nil
}

func (lock *EtcdRWLock) Key() string {
	return lock.key
}

func (lock *EtcdRWLock) RLock(ctx context.Context) error {
	etcdCli := global.GetEtcdManager()
	ctxNc := context.WithoutCancel(ctx)

	if err := lock.m3.Lock(ctx); err != nil {
		if err == context.Canceled {
			return nil // Never went out of the equilibrium state
		}
		return err
	}
	defer unlock(ctxNc, lock.m3)

	if err := lock.r.Lock(ctx); err != nil {
		if err == context.Canceled {
			return nil // Equilibrium state is reached by previous defered operations
		}
		return err
	}
	defer unlock(ctxNc, lock.r)

	if err := lock.m1.Lock(ctx); err != nil {
		if err == context.Canceled {
			return nil // Equilibrium state is reached by previous defered operations
		}
		return err
	}
	defer unlock(ctxNc, lock.m1)

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		if err == context.Canceled {
			return nil // Equilibrium state is reached by previous defered operations
		}
		return err
	}
	var readCounter int
	switch len(res.Kvs) {
	case 0:
		readCounter = 0
	case 1:
		str := string(res.Kvs[0].Value)
		readCounter, err = strconv.Atoi(str)
		if err != nil {
			return errors.New("invalid format for " + k + ", got " + str)
		}
	default:
		return errors.New("invalid etcd filter for " + k)
	}
	readCounter++
	_, err = etcdCli.Put(ctx, k, strconv.Itoa(readCounter))
	if err != nil {
		// Committed no value to etcd so it's fine.
		if err == context.Canceled {
			return nil // Equilibrium state is reached by previous defered operations
		}
		return err
	}

	// From now on, we cannot go back and need to finish the job, else way deadlock

	if readCounter == 1 {
		if err := lock.w.Lock(ctxNc); err != nil {
			if err == context.Canceled {
				return nil
			}
			return err
		}
	}

	return nil
}

func (lock *EtcdRWLock) RUnlock(ctx context.Context) error {
	etcdCli := global.GetEtcdManager()
	ctxNc := context.WithoutCancel(ctx)

	if err := lock.m1.Lock(ctx); err != nil {
		if err == context.Canceled {
			return nil // Never went out of the equilibrium state
		}
		return err
	}
	defer unlock(ctxNc, lock.m1)

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		if err == context.Canceled {
			return nil // Equilibrium state is reached by previous defered operations
		}
		return err
	}
	var readCounter int
	switch len(res.Kvs) {
	case 1:
		str := string(res.Kvs[0].Value)
		readCounter, err = strconv.Atoi(str)
		if err != nil {
			// Equilibrium state is natievly reached by previous defered operations
			return errors.New("invalid format for " + k + ", got " + str)
		}
	default:
		// Equilibrium state is natievly reached by previous defered operations
		return errors.New("invalid etcd filter for " + k)
	}
	readCounter--
	_, err = etcdCli.Put(ctx, k, strconv.Itoa(readCounter))
	if err != nil {
		// Committed no value to etcd so it's fine.
		// Defered functions will reach the equilibrium state
		if err == context.Canceled {
			return nil
		}
		return err
	}

	if readCounter == 0 {
		// Now that we wrote the readcounter, we can't skip the unlock else deadlock
		if err := lock.w.Unlock(ctxNc); err != nil {
			return err // can't be a context.Canceled as context is uncancelable
		}
	}

	return nil
}

func (lock *EtcdRWLock) RWLock(ctx context.Context) error {
	etcdCli := global.GetEtcdManager()
	ctxNc := context.WithoutCancel(ctx)

	if err := lock.m2.Lock(ctx); err != nil {
		if err == context.Canceled {
			return nil // Never went out of the equilibrium state
		}
		return err
	}

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		// Manually reach equilibrium state
		if err == context.Canceled {
			return lock.m2.Unlock(ctxNc)
		}
		return multierr.Combine(err, lock.m2.Unlock(ctxNc))
	}
	var writeCounter int
	switch len(res.Kvs) {
	case 0:
		writeCounter = 0
	case 1:
		str := string(res.Kvs[0].Value)
		writeCounter, err = strconv.Atoi(str)
		if err != nil {
			return multierr.Combine(
				errors.New("invalid format for "+k+", got "+str),
				lock.m2.Unlock(ctxNc), // Manually reach equilibrium state
			)
		}
	default:
		return multierr.Combine(
			errors.New("invalid etcd filter for "+k),
			lock.m2.Unlock(ctxNc), // Manually reach equilibrium state
		)
	}
	writeCounter++
	_, perr := etcdCli.Put(ctx, k, strconv.Itoa(writeCounter))
	if perr != nil {
		// Committed no value to etcd so it's fine.
		// Defered functions will reach the equilibrium state
		return multierr.Combine(
			err,
			lock.m2.Unlock(ctxNc), // Manually reach equilibrium state
		)
	}

	if writeCounter == 1 {
		// Now that we wrote the writecounter, we can't skip the lock else deadlock
		if err := lock.r.Lock(ctxNc); err != nil {
			return multierr.Combine(
				err,
				lock.m2.Unlock(ctxNc), // Manually reach equilibrium state
				lock.w.Lock(ctxNc),    // And don't forget we need to lock W to avoid deadlock
			)
		}
	}

	return multierr.Combine(
		lock.m2.Unlock(ctxNc),
		lock.w.Lock(ctxNc),
	)
}

func (lock *EtcdRWLock) RWUnlock(ctx context.Context) error {
	etcdCli := global.GetEtcdManager()
	ctxNc := context.WithoutCancel(ctx)

	// We cannot start by V(w) as in Courtois et al. paper, as if something goes wrong
	// we might be tempted to recover using P(w).
	//
	// Nonetheless, we have no guarantee that re-locking w will end shortly, thus
	// might starve indefinitely without possibility to complete request handling...
	// Then, we consider this operation unrecoverable hence perform it at last.
	//
	// This does not invalidate the Courtois et al. paper, simply reconsider unrelated
	// (in the meaning of involved locks and values) steps that are less efficient in
	// time to profit recoverability.

	if err := lock.m2.Lock(ctx); err != nil {
		if err == context.Canceled {
			return nil // Never went out of the equilibrium state
		}
		return err
	}

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		// Manually reach equilibrium state
		if err == context.Canceled {
			return lock.m2.Unlock(ctxNc)
		}
		return multierr.Combine(err, lock.m2.Unlock(ctxNc))
	}
	var writeCounter int
	switch len(res.Kvs) {
	case 1:
		str := string(res.Kvs[0].Value)
		writeCounter, err = strconv.Atoi(str)
		if err != nil {
			return multierr.Combine(
				errors.New("invalid format for "+k+", got "+str),
				lock.m2.Unlock(ctxNc), // Manually reach equilibrium state
			)
		}
	default:
		return multierr.Combine(
			errors.New("invalid etcd filter for "+k),
			lock.m2.Unlock(ctxNc), // Manually reach equilibrium state
		)
	}
	writeCounter--
	_, err = etcdCli.Put(ctx, k, strconv.Itoa(writeCounter))
	if err != nil {
		// Commited no value to etcd so it's fine.
		// Manually reach equilibrium state
		if err == context.Canceled {
			return lock.m2.Unlock(ctxNc)
		}
		return multierr.Combine(
			err,
			lock.m2.Unlock(ctxNc),
		)
	}

	if writeCounter == 0 {
		// Now that we wrote the writecounter, we can't skip the unlock else deadlock
		if err := lock.r.Unlock(ctxNc); err != nil {
			return multierr.Combine(
				err,
				lock.m2.Unlock(ctxNc),
			)
		}
	}

	// Don't forget the unrecoverable V(w) we discussed at the very beginning, we
	// here need to do it.
	// As we reached the critical section we MUST commit this change.
	return lock.w.Unlock(ctxNc)
}

func (lock *EtcdRWLock) Close() error {
	return lock.s.Close()
}

func unlock(ctx context.Context, mx *concurrency.Mutex) {
	if err := mx.Unlock(ctx); err != nil {
		global.Log().Error(ctx, "failed to unlock etcd mutex",
			zap.Error(err),
			zap.String("key", mx.Key()),
		)
	}
}
