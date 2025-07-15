package global

import (
	"sync"

	"github.com/ctfer-io/chall-manager/pkg/services/etcd"
	"go.uber.org/zap"
)

var (
	etcdInstance *etcd.Manager
	etcdOnce     sync.Once
)

func GetEtcdManager() *etcd.Manager {
	etcdOnce.Do(func() {
		etcdInstance = etcd.NewManager(etcd.Config{
			Endpoint: Conf.Etcd.Endpoint,
			Username: Conf.Etcd.Username,
			Password: Conf.Etcd.Password,
			Logger:   zap.NewNop(), // drop logs
			Tracer:   Tracer,       // inject the global OTel tracer
		})
	})
	return etcdInstance
}
