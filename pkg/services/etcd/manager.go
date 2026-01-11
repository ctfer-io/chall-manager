package etcd

import (
	"context"
	"sync"
	"time"

	"github.com/ctfer-io/chall-manager/pkg/otel"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type Manager struct {
	// Internal etcd state management
	mu     sync.RWMutex
	client *clientv3.Client
	config Config

	// Session management
	session *concurrency.Session
	gen     uint64

	// Healthcheck management
	hcMu      sync.Mutex // -> different from mu to protect healthcheck timing state
	lastHc    time.Time
	lastHcErr error
}

type Config struct {
	Endpoint string
	Username string
	Password string
	Logger   *zap.Logger
	Tracer   trace.Tracer
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
		if err := m.healthcheck(ctx, cli); err == nil {
			return cli, nil
		}
	}

	return m.recreateClient(ctx)
}

func (m *Manager) recreateClient(ctx context.Context) (*clientv3.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		if err := m.healthcheck(ctx, m.client); err == nil {
			return m.client, nil
		}
		// Close previous client and reset it
		_ = m.client.Close()
		m.client = nil

		// Reset session management
		m.session = nil

		// Reset healthcheck management
		m.hcMu.Lock()
		m.lastHc = time.Time{}
		m.lastHcErr = nil
		m.hcMu.Unlock()
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints: []string{m.config.Endpoint},
		Username:  m.config.Username,
		Password:  m.config.Password,
		Logger:    m.config.Logger,
		DialOptions: []grpc.DialOption{
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			grpc.WithUnaryInterceptor(otel.UnaryClientInterceptorWithCaller(m.config.Tracer)),
			grpc.WithStreamInterceptor(otel.StreamClientInterceptorWithCaller(m.config.Tracer)),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                30 * time.Second,
				Timeout:             5 * time.Second,
				PermitWithoutStream: true, // keep interacting to keep liveness
			}),
		},
	})
	if err != nil {
		return nil, err
	}

	// Opening a new session also behave as a healthcheck

	sess, err := concurrency.NewSession(cli)
	if err != nil {
		return nil, err
	}

	m.client = cli
	m.session = sess
	m.gen++ // every session has a unique generation number
	return cli, nil
}

func (m *Manager) GetSession(ctx context.Context) (*concurrency.Session, uint64, error) {
	// Get the client to ensure a session exist or is being recreated through m.recreateClient
	_, err := m.getClient(context.WithoutCancel(ctx)) // avoid cancelation as we'll reuse the client if recreated
	if err != nil {
		return nil, 0, err
	}
	return m.session, m.gen, nil
}

func (m *Manager) Get(ctx context.Context, k string) (*clientv3.GetResponse, error) {
	cli, err := m.getClient(context.WithoutCancel(ctx)) // avoid cancelation as we'll reuse the client if recreated
	if err != nil {
		return nil, err
	}
	return cli.Get(ctx, k)
}

func (m *Manager) Put(ctx context.Context, k, v string) (*clientv3.PutResponse, error) {
	cli, err := m.getClient(context.WithoutCancel(ctx)) // avoid cancelation as we'll reuse the client if recreated
	if err != nil {
		return nil, err
	}
	return cli.Put(ctx, k, v)
}

func (m *Manager) Healthcheck(ctx context.Context) error {
	_, err := m.getClient(context.WithoutCancel(ctx)) // avoid cancelation as we'll reuse the client if recreated
	return err
}

// A window of 10 seconds is quite low, but enough to avoid storming etcd with Get RPCs under high load.
const healthcheckWindow = 10 * time.Second

// healthcheck performs a Get on a random key. This is required to rotate authentication token if Chall-Manager is
// being unused more than the auth token's TTL.
// This principle is borrowed from `etcdctl endpoint health`.
func (m *Manager) healthcheck(ctx context.Context, cli *clientv3.Client) error {
	now := time.Now()
	m.hcMu.Lock()
	defer m.hcMu.Unlock()

	// Fast path: recent successful check
	if now.Sub(m.lastHc) < healthcheckWindow && m.lastHcErr == nil {
		return nil
	}

	// Slow path: actually hit etcd
	_, err := cli.Get(ctx, "health", clientv3.WithLimit(1))

	m.lastHc = time.Now()
	m.lastHcErr = err

	return err
}

func (m *Manager) Close(ctx context.Context) error {
	cli, err := m.getClient(context.WithoutCancel(ctx)) // avoid cancelation as we'll reuse the client if recreated
	if err != nil {
		return err
	}
	return multierr.Combine(
		m.session.Close(),
		cli.Close(),
	)
}
