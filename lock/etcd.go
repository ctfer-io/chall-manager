package lock

import (
	"context"

	"github.com/ctfer-io/chall-manager/global"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// The etcd distributed lock enables you to have a powerfull mutual exclusion (mutex)
// system in Kubernetes-based environment.
// You can't use the flock in Kubernetes as the Pods are isolated in there own contexts,
// hence don't share the filelock information.
func etcd(ctx context.Context, identity string) (func() error, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints: global.Conf.Lock.EtcdEndpoints,
		Username:  global.Conf.Lock.EtcdUsername,
		Password:  global.Conf.Lock.EtcdPassword,
		Logger:    global.Log(),
	})
	if err != nil {
		return nil, err
	}

	s, err := concurrency.NewSession(cli, concurrency.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	mx := concurrency.NewMutex(s, "/chall-manager/"+identity)
	if err := mx.Lock(ctx); err != nil {
		return nil, err
	}

	return func() error {
		if err := mx.Unlock(ctx); err != nil {
			return err
		}
		if err := s.Close(); err != nil {
			return err
		}
		if err := cli.Close(); err != nil {
			return err
		}
		return nil
	}, nil
}
