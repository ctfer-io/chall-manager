package etcd

import (
	"context"
	"sync"

	"github.com/sony/gobreaker/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Manager struct {
	mu     sync.RWMutex
	client *clientv3.Client
	config Config

	breaker *gobreaker.CircuitBreaker[any]
}

type Config struct {
	Endpoint string
	Username string
	Password string
	Logger   *zap.Logger

	CBOnStateChange func(name string, from, to gobreaker.State)
}

func NewManager(config Config) *Manager {
	return &Manager{
		config: config,
		breaker: gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
			Name:          "etcd circuit breaker",
			OnStateChange: config.CBOnStateChange,
		}),
	}
}

func (m *Manager) getClient(ctx context.Context) (*clientv3.Client, error) {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()

	if client != nil {
		if _, err := client.Status(ctx, m.config.Endpoint); err == nil {
			return client, nil
		}
	}

	return m.recreateClient(ctx)
}

func (m *Manager) recreateClient(ctx context.Context) (*clientv3.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		if _, err := m.client.Status(ctx, m.config.Endpoint); err == nil {
			return m.client, nil
		}
		_ = m.client.Close()
		m.client = nil
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints: []string{m.config.Endpoint},
		Username:  m.config.Username,
		Password:  m.config.Password,
		Logger:    m.config.Logger,
		DialOptions: []grpc.DialOption{
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		},
	})
	if err != nil {
		return nil, err
	}

	if _, err := cli.Status(ctx, m.config.Endpoint); err != nil {
		_ = cli.Close()
		return nil, err
	}

	m.client = cli
	return cli, nil
}

func (m *Manager) NewConcurrencySession(ctx context.Context) (*concurrency.Session, error) {
	cli, err := m.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return concurrency.NewSession(cli)
}

func (m *Manager) Get(ctx context.Context, k string) (*clientv3.GetResponse, error) {
	cli, err := m.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return cli.Get(ctx, k)
}

func (m *Manager) Put(ctx context.Context, k, v string) (*clientv3.PutResponse, error) {
	cli, err := m.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return cli.Put(ctx, k, v)
}

func (m *Manager) Healthcheck(ctx context.Context) error {
	cli, err := m.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err = cli.Status(ctx, m.config.Endpoint); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Close(ctx context.Context) error {
	cli, err := m.getClient(ctx)
	if err != nil {
		return err
	}
	return cli.Close()
}
