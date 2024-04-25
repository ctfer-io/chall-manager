package lock

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/ctfer-io/chall-manager/global"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/multierr"
)

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
//
// TODO improve error handling, especially of the defered function calls
type EtcdRWLock struct {
	key string
	cli *clientv3.Client
	s   *concurrency.Session

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
	cli, err := clientv3.New(clientv3.Config{
		Endpoints: global.Conf.Lock.EtcdEndpoints,
		Username:  global.Conf.Lock.EtcdUsername,
		Password:  global.Conf.Lock.EtcdPassword,
		Logger:    global.Log(),
	})
	if err != nil {
		return nil, err
	}

	s, err := concurrency.NewSession(cli)
	if err != nil {
		return nil, err
	}

	pfx := "/chall-manager/" + key + "/"
	return &EtcdRWLock{
		key: key,
		cli: cli,
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

func (lock *EtcdRWLock) RLock() error {
	ctx := lock.cli.Ctx()

	lock.m3.Lock(ctx)
	defer lock.m3.Unlock(ctx)

	lock.r.Lock(ctx)
	defer lock.r.Unlock(ctx)

	lock.m1.Lock(ctx)
	defer lock.m1.Unlock(ctx)

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := lock.cli.Get(ctx, k)
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
	lock.cli.Put(ctx, k, strconv.Itoa(readCounter))

	if readCounter == 1 {
		lock.w.Lock(ctx)
	}

	return nil
}

func (lock *EtcdRWLock) RUnlock() error {
	ctx := lock.cli.Ctx()

	lock.m1.Lock(ctx)
	defer lock.m1.Unlock(ctx)

	k := fmt.Sprintf("/chall-manager/%s/readCounter", lock.key)
	res, err := lock.cli.Get(lock.cli.Ctx(), k)
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
	lock.cli.Put(lock.cli.Ctx(), k, strconv.Itoa(readCounter))

	if readCounter == 0 {
		lock.w.Unlock(ctx)
	}

	return nil
}

func (lock *EtcdRWLock) RWLock() error {
	ctx := lock.cli.Ctx()

	defer lock.w.Lock(ctx)

	lock.m2.Lock(ctx)
	defer lock.m2.Unlock(ctx)

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := lock.cli.Get(lock.cli.Ctx(), k)
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
	lock.cli.Put(lock.cli.Ctx(), k, strconv.Itoa(writeCounter))

	if writeCounter == 1 {
		lock.r.Lock(ctx)
	}

	return nil
}

func (lock *EtcdRWLock) RWUnlock() error {
	ctx := lock.cli.Ctx()

	lock.w.Unlock(ctx)

	lock.m2.Lock(ctx)
	defer lock.m2.Unlock(ctx)

	k := fmt.Sprintf("/chall-manager/%s/writeCounter", lock.key)
	res, err := lock.cli.Get(lock.cli.Ctx(), k)
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
	lock.cli.Put(lock.cli.Ctx(), k, strconv.Itoa(writeCounter))

	if writeCounter == 1 {
		lock.r.Unlock(ctx)
	}

	return nil
}

func (lock *EtcdRWLock) Close() error {
	return multierr.Combine(
		lock.s.Close(),
		lock.cli.Close(),
	)
}
