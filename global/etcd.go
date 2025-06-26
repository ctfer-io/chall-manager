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
			Endpoint: Conf.Lock.EtcdEndpoints[0], // XXX this support only one endpoint for now
			Username: Conf.Lock.EtcdUsername,
			Password: Conf.Lock.EtcdPassword,
			Logger:   zap.NewNop(), // drop logs
			// TODO add CBOnStateChange
		})
	})
	return etcdInstance
}
