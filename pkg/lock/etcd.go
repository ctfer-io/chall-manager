package lock

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/multierr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EtcdRWLock is a custom implementation of a reader-writer writer-preference (often referred to as "problem 2")
// concurrent control strategy, based upon etcd distributed values and mutual exclusions (mutex).
//
// For error management purposes, it does not use "defer" instructions thus any error is processed synchronously.
//
// Current implementation expect the network to be reliable.
// Moreover, it is unqueued thus unfair, and does not repair in case of session rotation.
//
// It tries its best to ensure each operation is atomic, by executing actions that can be recovered easily.
// Such is performed through the algorithm:
//   - Create a "stack" of previous P(x) for their corresponding V(x), and add V(x) iif x should be recoverable;
//   - If the V(x) is executed in the normal workflow, it is poped out of the "stack" as it is not longer needed
//     to recover;
//   - If there is an error, execute all stack in LIFO order.
//
// By doing so, we expect an operation to recover even if it is canceled by the requester.
// Note that it behave somewhat similar to sagas-based recovery, but not using them is simpler to model in our case.
// TODO make counters recoverable too to avoid deadlocks.
//
// Original implementation based upon 'Concurrent Control with "Readers" and "Writers"' by Courtois et al. (1971)
// (DOI: 10.1145/362759.362813).
// Implementations decisions with etcd in the context of distributed systems goes to CTFer.io.
type EtcdRWLock struct {
	key string
	gen uint64 // track which session it comes from, avoid freeing already-free locks

	// readCounter  -> /chall-manager/<key>/readCounter
	// writeCounter -> /chall-manager/<key>/writeCounter
	m1, m2, m3, r, w *concurrency.Mutex
	// m1 -> /chall-manager/<key>/m1
	// m2 -> /chall-manager/<key>/m2
	// "[m3] prevents too many readers from waiting on mutex r, so writers have a good chance to signal r when they
	// come", from user "Attala" on Stackoverflow.
	// Ref: https://stackoverflow.com/questions/9974384/second-algorithm-solution-to-readers-writer
	// m3 -> /chall-manager/<key>/m3
	// r  -> /chall-manager/<key>/r
	// w  -> /chall-manager/<key>/w
}

func NewEtcdRWLock(ctx context.Context, key string) (RWLock, error) {
	s, gen, err := global.GetEtcdManager().GetSession(ctx)
	if err != nil {
		return nil, err
	}

	pfx := "/chall-manager/" + key + "/"
	return &EtcdRWLock{
		key: key,
		gen: gen,
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
		if lock.IsCanceled(err) {
			return nil // Never went out of the equilibrium state
		}
		return errors.Wrap(err, "m3 lock")
	}

	if err := lock.r.Lock(ctx); err != nil {
		if lock.IsCanceled(err) {
			return lock.unlock(ctxNc, lock.m3, "m3") // Equilibrium state is reached by releasing acquired m3
		}
		return multierr.Combine(
			errors.Wrap(err, "r lock"),
			lock.unlock(ctxNc, lock.m3, "m3"),
		)
	}

	if err := lock.m1.Lock(ctx); err != nil {
		if lock.IsCanceled(err) {
			return multierr.Combine( // Equilibrium state is reached by releasing acquired r then m3 (LIFO)
				lock.unlock(ctxNc, lock.r, "r"),
				lock.unlock(ctxNc, lock.m3, "m3"),
			)
		}
		return multierr.Combine(
			errors.Wrap(err, "m1 lock"),
			lock.unlock(ctxNc, lock.r, "r"),
			lock.unlock(ctxNc, lock.m3, "m3"),
		)
	}

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		if lock.IsCanceled(err) {
			return multierr.Combine( // Equilibrium state is reached by releasing acquired m1 then r then m3 (LIFO)
				lock.unlock(ctxNc, lock.m1, "m1"),
				lock.unlock(ctxNc, lock.r, "r"),
				lock.unlock(ctxNc, lock.m3, "m3"),
			)
		}
		return multierr.Combine(
			errors.Wrap(err, "get"),
			lock.unlock(ctxNc, lock.m1, "m1"),
			lock.unlock(ctxNc, lock.r, "r"),
			lock.unlock(ctxNc, lock.m3, "m3"),
		)
	}
	var readCounter int
	switch len(res.Kvs) {
	case 0:
		readCounter = 0
	case 1:
		str := string(res.Kvs[0].Value)
		readCounter, err = strconv.Atoi(str)
		if err != nil {
			return multierr.Combine( // Equilibrium state is reached by releasing acquired m1 then r then m3 (LIFO)
				errors.New("invalid format for "+k+", got "+str),
				lock.unlock(ctxNc, lock.m1, "m1"),
				lock.unlock(ctxNc, lock.r, "r"),
				lock.unlock(ctxNc, lock.m3, "m3"),
			)
		}
	default:
		return multierr.Combine( // Equilibrium state is reached by releasing acquired m1 then r then m3 (LIFO)
			errors.New("invalid etcd filter for "+k),
			lock.unlock(ctxNc, lock.m1, "m1"),
			lock.unlock(ctxNc, lock.r, "r"),
			lock.unlock(ctxNc, lock.m3, "m3"),
		)
	}
	readCounter++
	_, err = etcdCli.Put(ctx, k, strconv.Itoa(readCounter))
	if err != nil {
		// Committed no value to etcd so it's fine.
		if lock.IsCanceled(err) {
			return multierr.Combine( // Equilibrium state is reached by releasing acquired m1 then r then m3 (LIFO)
				lock.unlock(ctxNc, lock.m1, "m1"),
				lock.unlock(ctxNc, lock.r, "r"),
				lock.unlock(ctxNc, lock.m3, "m3"),
			)
		}
		return multierr.Combine(
			errors.Wrap(err, "put"),
			lock.unlock(ctxNc, lock.m1, "m1"),
			lock.unlock(ctxNc, lock.r, "r"),
			lock.unlock(ctxNc, lock.m3, "m3"),
		)
	}

	// From now on, we cannot go back and need to finish the job, else way there will be a deadlock

	if readCounter == 1 {
		if err := lock.w.Lock(ctxNc); err != nil {
			if lock.IsCanceled(err) {
				return multierr.Combine( // Equilibrium state is reached by releasing acquired m1 then r then m3 (LIFO)
					lock.unlock(ctxNc, lock.m1, "m1"),
					lock.unlock(ctxNc, lock.r, "r"),
					lock.unlock(ctxNc, lock.m3, "m3"),
				)
			}
			return multierr.Combine(
				errors.Wrap(err, "w lock"),
				lock.unlock(ctxNc, lock.m1, "m1"),
				lock.unlock(ctxNc, lock.r, "r"),
				lock.unlock(ctxNc, lock.m3, "m3"),
			)
		}
	}

	if err := lock.unlock(ctxNc, lock.m1, "m1"); err != nil {
		return multierr.Combine(
			err,
			lock.unlock(ctxNc, lock.r, "r"),
			lock.unlock(ctxNc, lock.m3, "m3"),
		)
	}

	if err := lock.unlock(ctxNc, lock.r, "r"); err != nil {
		return multierr.Combine(
			err,
			lock.unlock(ctxNc, lock.m3, "m3"),
		)
	}

	return lock.unlock(ctxNc, lock.m3, "m3")
}

func (lock *EtcdRWLock) RUnlock(ctx context.Context) error {
	etcdCli := global.GetEtcdManager()
	ctxNc := context.WithoutCancel(ctx)

	if err := lock.m1.Lock(ctx); err != nil {
		if lock.IsCanceled(err) {
			return nil // Never went out of the equilibrium state
		}
		return errors.Wrap(err, "m1 lock")
	}

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		if lock.IsCanceled(err) {
			return lock.unlock(ctxNc, lock.m1, "m1") // Equilibrium state is reached by releasing acquired m1
		}
		return multierr.Combine(
			err,
			lock.unlock(ctxNc, lock.m1, "m1"),
		)
	}
	var readCounter int
	switch len(res.Kvs) {
	case 1:
		str := string(res.Kvs[0].Value)
		readCounter, err = strconv.Atoi(str)
		if err != nil {
			// Equilibrium state is reached by releasing acquired m1
			return multierr.Combine(
				errors.New("invalid format for "+k+", got "+str),
				lock.unlock(ctxNc, lock.m1, "m1"),
			)
		}
	default:
		// Equilibrium state is reached by releasing acquired m1
		return multierr.Combine(
			errors.New("invalid etcd filter for "+k),
			lock.unlock(ctxNc, lock.m1, "m1"),
		)
	}
	readCounter--
	_, err = etcdCli.Put(ctx, k, strconv.Itoa(readCounter))
	if err != nil {
		// Committed no value to etcd so it's fine.
		// Equilibrium state is reached by releasing acquired m1
		if lock.IsCanceled(err) {
			return lock.unlock(ctxNc, lock.m1, "m1")
		}
		return multierr.Combine(
			err,
			lock.unlock(ctxNc, lock.m1, "m1"),
		)
	}

	// From now on, we cannot go back and need to finish the job, else way there will be a deadlock

	if readCounter == 0 {
		// Now that we wrote the readcounter, we can't skip the unlock else deadlock
		if err := lock.unlock(ctxNc, lock.w, "w"); err != nil {
			return multierr.Combine(
				err,
				lock.unlock(ctxNc, lock.m1, "m1"),
			)
		}
	}

	return lock.unlock(ctxNc, lock.m1, "m1")
}

func (lock *EtcdRWLock) RWLock(ctx context.Context) error {
	etcdCli := global.GetEtcdManager()
	ctxNc := context.WithoutCancel(ctx)

	if err := lock.m2.Lock(ctx); err != nil {
		if lock.IsCanceled(err) {
			return nil // Never went out of the equilibrium state
		}
		return errors.Wrap(err, "m2 lock")
	}

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		// Manually reach equilibrium state
		if lock.IsCanceled(err) {
			return lock.unlock(ctxNc, lock.m2, "m2")
		}
		return multierr.Combine(err, lock.unlock(ctxNc, lock.m2, "m2"))
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
				lock.unlock(ctxNc, lock.m2, "m2"), // Manually reach equilibrium state
			)
		}
	default:
		return multierr.Combine(
			errors.New("invalid etcd filter for "+k),
			lock.unlock(ctxNc, lock.m2, "m2"), // Manually reach equilibrium state
		)
	}
	writeCounter++
	_, err = etcdCli.Put(ctx, k, strconv.Itoa(writeCounter))
	if err != nil {
		// Committed no value to etcd so it's fine.
		// Defered functions will reach the equilibrium state
		if lock.IsCanceled(err) {
			return lock.unlock(ctxNc, lock.m2, "m2")
		}
		return multierr.Combine(
			err,
			lock.unlock(ctxNc, lock.m2, "m2"), // Manually reach equilibrium state
		)
	}

	// From now on, we cannot go back and need to finish the job, else way there will be a deadlock

	if writeCounter == 1 {
		// Now that we wrote the writecounter, we can't skip the lock else deadlock
		if err := lock.r.Lock(ctxNc); err != nil {
			return multierr.Combine(
				errors.Wrap(err, "r lock"),
				lock.unlock(ctxNc, lock.m2, "m2"), // Manually reach equilibrium state
				lock.w.Lock(ctxNc),                // And don't forget we need to lock W to avoid deadlock
			)
		}
	}

	return multierr.Combine(
		lock.unlock(ctxNc, lock.m2, "m2"),
		lock.w.Lock(ctxNc),
	)
}

func (lock *EtcdRWLock) RWUnlock(ctx context.Context) error {
	etcdCli := global.GetEtcdManager()
	ctxNc := context.WithoutCancel(ctx)

	// We cannot start by V(w) as in Courtois et al. paper, as if something goes wrong we might be tempted to recover
	// using P(w).
	//
	// Nonetheless, we have no guarantee that re-locking w will end shortly, thus might starve indefinitely without
	// possibility to complete request handling...
	// Then, we consider this operation unrecoverable hence perform it at last.
	//
	// This does not invalidate the Courtois et al. paper, simply reconsider unrelated (in the meaning of involved
	// locks and values) steps that are less efficient in time to profit recoverability.

	if err := lock.m2.Lock(ctx); err != nil {
		if lock.IsCanceled(err) {
			return nil // Never went out of the equilibrium state
		}
		return errors.Wrap(err, "m2 lock")
	}

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
		// Manually reach equilibrium state
		if lock.IsCanceled(err) {
			return lock.unlock(ctxNc, lock.m2, "m2")
		}
		return multierr.Combine(err, lock.unlock(ctxNc, lock.m2, "m2"))
	}
	var writeCounter int
	switch len(res.Kvs) {
	case 1:
		str := string(res.Kvs[0].Value)
		writeCounter, err = strconv.Atoi(str)
		if err != nil {
			return multierr.Combine(
				errors.New("invalid format for "+k+", got "+str),
				lock.unlock(ctxNc, lock.m2, "m2"), // Manually reach equilibrium state
			)
		}
	default:
		return multierr.Combine(
			errors.New("invalid etcd filter for "+k),
			lock.unlock(ctxNc, lock.m2, "m2"), // Manually reach equilibrium state
		)
	}
	writeCounter--
	_, err = etcdCli.Put(ctx, k, strconv.Itoa(writeCounter))
	if err != nil {
		// Committed no value to etcd so it's fine.
		// Manually reach equilibrium state
		if lock.IsCanceled(err) {
			return lock.unlock(ctxNc, lock.m2, "m2")
		}
		return multierr.Combine(
			err,
			lock.unlock(ctxNc, lock.m2, "m2"),
		)
	}

	// From now on, we cannot go back and need to finish the job, else way there will be a deadlock

	if writeCounter == 0 {
		// Now that we wrote the writecounter, we can't skip the unlock else deadlock
		if err := lock.unlock(ctxNc, lock.r, "r"); err != nil {
			return multierr.Combine(
				err,
				lock.unlock(ctxNc, lock.m2, "m2"),
				lock.unlock(ctxNc, lock.w, "w"),
			)
		}
	}

	if err := lock.unlock(ctxNc, lock.m2, "m2"); err != nil {
		return multierr.Combine(
			err,
			lock.unlock(ctxNc, lock.w, "w"),
		)
	}

	// Don't forget the unrecoverable V(w) we discussed at the very beginning, we
	// here need to do it.
	// As we reached the critical section we MUST commit this change.
	// if err := lock.unlock(ctxNc, lock.w, "w"); err != nil {
	if err := lock.unlock(ctxNc, lock.w, "w"); err != nil {
		return err
	}
	return nil
}

// Unlocking with etcd is kinda specific, as it depends on the session.
// This last is handled by the etcd manager.
// This function is a helper for fault-resilient unlocking that copy with already-closed or closing sessions.
//
// When a session is rotated (can happen under load), it releases all the locks it held until now thus we need to make
// our functions recoverable natively. There might be issues, but at least we must ensure there is no deadlock!
//
// All calls use cancel-free context so it's fine wrapping errors for additional data on which mutex produced an error.
func (lock *EtcdRWLock) unlock(ctx context.Context, mx *concurrency.Mutex, name string) error {
	// If the session has rotated, the locks were freed so cannot free them again (else way it is a double-free)
	// and we'll see errors.
	if lock.stillValid(ctx) {
		return nil
	}

	// Unlock the mutex, and if no error keep going
	err := mx.Unlock(ctx)
	if err == nil {
		return nil
	}

	// If the session is still valid AND there was an issue (watch out for the context cancelation) then it is
	// a valid error.
	if lock.stillValid(ctx) {
		return errors.Wrap(err, name+" unlock")
	}

	// Finally, in the meantime of unlocking, the session rotated so the unlock is semantically invalid, yet the
	// operation is no-op so we're fine, no need to recover.
	return nil
}

func (lock *EtcdRWLock) stillValid(ctx context.Context) bool {
	_, gen, err := global.GetEtcdManager().GetSession(ctx)
	if err != nil {
		return false
	}
	return gen == lock.gen
}

func (*EtcdRWLock) IsCanceled(err error) bool {
	if err == context.Canceled {
		return true
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.Canceled
}
