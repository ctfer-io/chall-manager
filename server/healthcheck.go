package server

import (
	"context"
	"net/http"
	"time"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/hellofresh/health-go/v5"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
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

		// TODO connect this to the CircuitBreaker, and shares a global structure around this
		_ = h.Register(health.Config{
			Name:    "etcd",
			Timeout: time.Second,
			Check: func(ctx context.Context) error {
				_, err := clientv3.New(clientv3.Config{
					Context:   ctx,
					Endpoints: global.Conf.Lock.EtcdEndpoints,
					Username:  global.Conf.Lock.EtcdUsername,
					Password:  global.Conf.Lock.EtcdPassword,
					Logger:    global.Log().Sub,
					DialOptions: []grpc.DialOption{
						grpc.WithStatsHandler(otelgrpc.NewClientHandler()), // propagate OTEL span info
					},
				})
				return err
			},
			SkipOnErr: true,
		})
	}

	return h.Handler()
}
