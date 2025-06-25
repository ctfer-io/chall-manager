package server

import (
	"context"
	"net/http"
	"time"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/hellofresh/health-go/v5"
	"go.uber.org/zap"
)

func healthcheck(ctx context.Context) http.Handler {
	opts := []health.Option{
		health.WithComponent(health.Component{
			Name:    "chall-manager",
			Version: global.Version,
		}),
		health.WithSystemInfo(),
	}
	h, err := health.New(opts...)
	if err != nil {
		panic(err)
	}

	if len(global.Conf.Lock.EtcdEndpoints) != 0 {
		global.Log().Info(ctx, "registering healthcheck config",
			zap.String("service", "etcd"),
		)

		_ = h.Register(health.Config{
			Name:    "etcd",
			Timeout: time.Second,
			Check: func(ctx context.Context) error {
				man := global.GetEtcdManager()
				return man.Healthcheck(ctx)
			},
		})
	}

	return h.Handler()
}
