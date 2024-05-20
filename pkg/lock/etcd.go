package lock

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/ctfer-io/chall-manager/global"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

var (
	etcdCli  *clientv3.Client
	etcdOnce sync.Once
)

// TODO plug it to a circuit breaker to improve resiliency
func getClient() *clientv3.Client {
	etcdOnce.Do(func() {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints: global.Conf.Lock.EtcdEndpoints,
			Username:  global.Conf.Lock.EtcdUsername,
			Password:  global.Conf.Lock.EtcdPassword,
			Logger:    global.Log(),
		})
		if err != nil {
			panic("failed to init etcd client: " + err.Error())
		}
		etcdCli = cli
	})
	return etcdCli
}

// The etcd distributed lock enables you to have a powerfull mutual exclusion (mutex)
// system in Kubernetes-based environment.
// You can't use the flock in Kubernetes as the Pods are isolated in there own contexts,
// hence don't share the filelock information.
//
// This implementation goes further than a simple mutex, as it implements the
// readers-writer lock for a writer-preference.
//
// Based upon 'Concurrent Control with "Readers" and "Writers"' by Courtois et al.,
// DOI: 10.1145/362759.362813
type EtcdRWLock struct {
	key string
	s   *concurrency.Session
	ctx context.Context

	// readCounter  -> /chall-manager/<key>/readCounter
	// writeCounter -> /chall-manager/<key>/writeCounter
	m1, m2, m3, r, w *concurrency.Mutex
	// m1 -> /chall-manager/<key>/m1
	// m2 -> /chall-manager/<key>/m2
	// m3 -> /chall-manager/<key>/m3
	// r  -> /chall-manager/<key>/r
	// w  -> /chall-manager/<key>/w
}

func NewEtcdRWLock(ctx context.Context, key string) (RWLock, error) {
	s, err := concurrency.NewSession(getClient())
	if err != nil {
		return nil, err
	}

	pfx := "/chall-manager/" + key + "/"
	return &EtcdRWLock{
		key: key,
		s:   s,
		ctx: ctx,
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

func (lock *EtcdRWLock) RLock() error {
	ctx := lock.ctx

	if err := lock.m3.Lock(ctx); err != nil {
		return err
	}
	defer unlock(ctx, lock.m3)

	if err := lock.r.Lock(ctx); err != nil {
		return err
	}
	defer unlock(ctx, lock.r)

	if err := lock.m1.Lock(ctx); err != nil {
		return err
	}
	defer unlock(ctx, lock.m1)

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := etcdCli.Get(ctx, k)
	if err != nil {
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
	_, perr := etcdCli.Put(ctx, k, strconv.Itoa(readCounter))
	// Don't return perr for now, let's avoid race conditions and starvations

	if readCounter == 1 {
		if err := lock.w.Lock(ctx); err != nil {
			return err
		}
	}

	return perr
}

func (lock *EtcdRWLock) RUnlock() error {
	ctx := lock.ctx

	if err := lock.m1.Lock(ctx); err != nil {
		return err
	}
	defer unlock(ctx, lock.m1)

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := etcdCli.Get(lock.ctx, k)
	if err != nil {
		return err
	}
	var readCounter int
	switch len(res.Kvs) {
	case 1:
		str := string(res.Kvs[0].Value)
		readCounter, err = strconv.Atoi(str)
		if err != nil {
			return errors.New("invalid format for " + k + ", got " + str)
		}
	default:
		return errors.New("invalid etcd filter for " + k)
	}
	readCounter--
	_, perr := etcdCli.Put(lock.ctx, k, strconv.Itoa(readCounter))
	// Don't return perr for now, let's avoid race conditions and starvations

	if readCounter == 0 {
		if err := lock.w.Unlock(ctx); err != nil {
			return err
		}
	}

	return perr
}

func (lock *EtcdRWLock) RWLock() error {
	ctx := lock.ctx

	defer func(ctx context.Context, mx *concurrency.Mutex) {
		if err := mx.Lock(ctx); err != nil {
			global.Log().Error("failed to lock etcd mutex",
				zap.Error(err),
				zap.String("key", mx.Key()),
			)
		}
	}(ctx, lock.w)

	if err := lock.m2.Lock(ctx); err != nil {
		return err
	}
	defer unlock(ctx, lock.m2)

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := etcdCli.Get(lock.ctx, k)
	if err != nil {
		return err
	}
	var writeCounter int
	switch len(res.Kvs) {
	case 0:
		writeCounter = 0
	case 1:
		str := string(res.Kvs[0].Value)
		writeCounter, err = strconv.Atoi(str)
		if err != nil {
			return errors.New("invalid format for " + k + ", got " + str)
		}
	default:
		return errors.New("invalid etcd filter for " + k)
	}
	writeCounter++
	_, perr := etcdCli.Put(lock.ctx, k, strconv.Itoa(writeCounter))
	// Don't return perr for now, let's avoid race conditions and starvations

	if writeCounter == 1 {
		if err := lock.r.Lock(ctx); err != nil {
			return err
		}
	}

	return perr
}

func (lock *EtcdRWLock) RWUnlock() error {
	ctx := lock.ctx

	if err := lock.w.Unlock(ctx); err != nil {
		return err
	}

	if err := lock.m2.Lock(ctx); err != nil {
		return err
	}
	defer unlock(ctx, lock.m2)

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := etcdCli.Get(lock.ctx, k)
	if err != nil {
		return err
	}
	var writeCounter int
	switch len(res.Kvs) {
	case 1:
		str := string(res.Kvs[0].Value)
		writeCounter, err = strconv.Atoi(str)
		if err != nil {
			return errors.New("invalid format for " + k + ", got " + str)
		}
	default:
		return errors.New("invalid etcd filter for " + k)
	}
	writeCounter--
	_, perr := etcdCli.Put(lock.ctx, k, strconv.Itoa(writeCounter))
	// Don't return perr for now, let's avoid race conditions and starvations

	if writeCounter == 1 {
		if err := lock.r.Unlock(ctx); err != nil {
			return err
		}
	}

	return perr
}

func (lock *EtcdRWLock) Close() error {
	return lock.s.Close()
}

func unlock(ctx context.Context, mx *concurrency.Mutex) {
	if err := mx.Unlock(ctx); err != nil {
		global.Log().Error("failed to unlock etcd mutex",
			zap.Error(err),
			zap.String("key", mx.Key()),
		)
	}
}
