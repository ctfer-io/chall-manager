package lock

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/sony/gobreaker/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	etcdCli  *clientv3.Client
	etcdOnce sync.Once
)

func getClient() *clientv3.Client {
	etcdOnce.Do(func() {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints: global.Conf.Lock.EtcdEndpoints,
			Username:  global.Conf.Lock.EtcdUsername,
			Password:  global.Conf.Lock.EtcdPassword,
			Logger:    zap.NewNop(), // Disable logger for etcd as it spams logs
			DialOptions: []grpc.DialOption{
				grpc.WithStatsHandler(otelgrpc.NewClientHandler()), // propagate OTEL span info
			},
		})
		if err != nil {
			panic("failed to init etcd client: " + err.Error())
		}
		etcdCli = cli
	})
	return etcdCli
}

// The etcd distributed lock enables you to have a powerful mutual exclusion (mutex)
// system in with an etcd cluster.
// You can't use a file lock in distributed environments such as Kubernetes because
// the Pods are isolated in there own contexts hence would not share the filelock
// information.
//
// This implementation goes further than a simple mutex, as it implements the
// readers-writer lock for a writer-preference.
// Moreover, it implements a circuit breaker under the hood to handle network failures
// smoothly.
//
// Based upon 'Concurrent Control with "Readers" and "Writers"' by Courtois et al. (1971)
// DOI: 10.1145/362759.362813
type EtcdRWLock struct {
	key string
	s   *concurrency.Session

	cb *gobreaker.CircuitBreaker[struct{}]

	// readCounter  -> /chall-manager/<key>/readCounter
	// writeCounter -> /chall-manager/<key>/writeCounter
	m1, m2, m3, r, w *concurrency.Mutex
	// m1 -> /chall-manager/<key>/m1
	// m2 -> /chall-manager/<key>/m2
	// m3 -> /chall-manager/<key>/m3
	// r  -> /chall-manager/<key>/r
	// w  -> /chall-manager/<key>/w
}

func NewEtcdRWLock(key string) (RWLock, error) {
	s, err := concurrency.NewSession(getClient())
	if err != nil {
		return nil, err
	}

	// TODO pipe CircuitBreaker status into telemetry data, follow Netflix Hystrix approach ?
	cb := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name: key,
	})

	pfx := "/chall-manager/" + key + "/"
	return &EtcdRWLock{
		key: key,
		s:   s,
		cb:  cb,
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
	_, err := lock.cb.Execute(func() (_ struct{}, err error) {
		if err = lock.m3.Lock(ctx); err != nil {
			return
		}
		defer unlock(ctx, lock.m3)

		if err = lock.r.Lock(ctx); err != nil {
			return
		}
		defer unlock(ctx, lock.r)

		if err = lock.m1.Lock(ctx); err != nil {
			return
		}
		defer unlock(ctx, lock.m1)

		k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
		res, err := etcdCli.Get(ctx, k)
		if err != nil {
			return
		}
		var readCounter int
		switch len(res.Kvs) {
		case 0:
			readCounter = 0
		case 1:
			str := string(res.Kvs[0].Value)
			readCounter, err = strconv.Atoi(str)
			if err != nil {
				err = errors.New("invalid format for " + k + ", got " + str)
				return
			}
		default:
			err = errors.New("invalid etcd filter for " + k)
			return
		}
		readCounter++
		_, perr := etcdCli.Put(ctx, k, strconv.Itoa(readCounter))
		// Don't return perr for now, let's avoid race conditions and starvations

		if readCounter == 1 {
			if err = lock.w.Lock(ctx); err != nil {
				return
			}
		}

		err = perr
		return
	})
	return err
}

func (lock *EtcdRWLock) RUnlock(ctx context.Context) error {
	_, err := lock.cb.Execute(func() (_ struct{}, err error) {
		if err = lock.m1.Lock(ctx); err != nil {
			return
		}
		defer unlock(ctx, lock.m1)

		k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
		res, err := etcdCli.Get(ctx, k)
		if err != nil {
			return
		}
		var readCounter int
		switch len(res.Kvs) {
		case 1:
			str := string(res.Kvs[0].Value)
			readCounter, err = strconv.Atoi(str)
			if err != nil {
				err = errors.New("invalid format for " + k + ", got " + str)
				return
			}
		default:
			err = errors.New("invalid etcd filter for " + k)
			return
		}
		readCounter--
		_, perr := etcdCli.Put(ctx, k, strconv.Itoa(readCounter))
		// Don't return perr for now, let's avoid race conditions and starvations

		if readCounter == 0 {
			if err = lock.w.Unlock(ctx); err != nil {
				return
			}
		}

		err = perr
		return
	})
	return err
}

func (lock *EtcdRWLock) RWLock(ctx context.Context) error {
	_, err := lock.cb.Execute(func() (_ struct{}, err error) {
		defer func(ctx context.Context, mx *concurrency.Mutex) {
			if err := mx.Lock(ctx); err != nil {
				global.Log().Error(ctx, "failed to lock etcd mutex",
					zap.Error(err),
					zap.String("key", mx.Key()),
				)
			}
		}(ctx, lock.w)

		if err = lock.m2.Lock(ctx); err != nil {
			return
		}
		defer unlock(ctx, lock.m2)

		k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
		res, err := etcdCli.Get(ctx, k)
		if err != nil {
			return
		}
		var writeCounter int
		switch len(res.Kvs) {
		case 0:
			writeCounter = 0
		case 1:
			str := string(res.Kvs[0].Value)
			writeCounter, err = strconv.Atoi(str)
			if err != nil {
				err = errors.New("invalid format for " + k + ", got " + str)
				return
			}
		default:
			err = errors.New("invalid etcd filter for " + k)
			return
		}
		writeCounter++
		_, perr := etcdCli.Put(ctx, k, strconv.Itoa(writeCounter))
		// Don't return perr for now, let's avoid race conditions and starvations

		if writeCounter == 1 {
			if err = lock.r.Lock(ctx); err != nil {
				return
			}
		}

		err = perr
		return
	})
	return err
}

func (lock *EtcdRWLock) RWUnlock(ctx context.Context) error {
	_, err := lock.cb.Execute(func() (_ struct{}, err error) {
		if err = lock.w.Unlock(ctx); err != nil {
			return
		}

		if err = lock.m2.Lock(ctx); err != nil {
			return
		}
		defer unlock(ctx, lock.m2)

		k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
		res, err := etcdCli.Get(ctx, k)
		if err != nil {
			return
		}
		var writeCounter int
		switch len(res.Kvs) {
		case 1:
			str := string(res.Kvs[0].Value)
			writeCounter, err = strconv.Atoi(str)
			if err != nil {
				err = errors.New("invalid format for " + k + ", got " + str)
				return
			}
		default:
			err = errors.New("invalid etcd filter for " + k)
			return
		}
		writeCounter--
		_, perr := etcdCli.Put(ctx, k, strconv.Itoa(writeCounter))
		// Don't return perr for now, let's avoid race conditions and starvations

		if writeCounter == 0 {
			if err = lock.r.Unlock(ctx); err != nil {
				return
			}
		}

		err = perr
		return
	})
	return err
}

func (lock *EtcdRWLock) Close() error {
	_, err := lock.cb.Execute(func() (struct{}, error) {
		return struct{}{}, lock.s.Close()
	})
	return err
}

func unlock(ctx context.Context, mx *concurrency.Mutex) {
	if err := mx.Unlock(ctx); err != nil {
		global.Log().Error(ctx, "failed to unlock etcd mutex",
			zap.Error(err),
			zap.String("key", mx.Key()),
		)
	}
}
