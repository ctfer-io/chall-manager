package etcd

import (
	"context"
	"sync"

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
}

type Config struct {
	Endpoint string
	Username string
	Password string
	Logger   *zap.Logger
}

func NewManager(config Config) *Manager {
	return &Manager{
		config: config,
	}
}

func (m *Manager) getClient(ctx context.Context) (*clientv3.Client, error) {
	m.mu.RLock()
	cli := m.client
	m.mu.RUnlock()

	if cli != nil {
		if err := healthcheck(ctx, cli); err == nil {
			return cli, nil
		}
	}

	return m.recreateClient(ctx)
}

func (m *Manager) recreateClient(ctx context.Context) (*clientv3.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		if err := healthcheck(ctx, m.client); err == nil {
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

	if err := healthcheck(ctx, cli); err != nil {
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
	return healthcheck(ctx, cli)
}

// healthcheck performs a Get on a random key.
// This principle is borrowed from `etcdctl endpoint health`.
func healthcheck(ctx context.Context, cli *clientv3.Client) error {
	_, err := cli.Get(ctx, "health")
	return err
}

func (m *Manager) Close(ctx context.Context) error {
	cli, err := m.getClient(ctx)
	if err != nil {
		return err
	}
	return cli.Close()
}
