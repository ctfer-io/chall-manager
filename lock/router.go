package lock

import (
	"context"

	"github.com/ctfer-io/chall-manager/global"
)

func Acquire(ctx context.Context, identity string) (release func() error, err error) {
	switch global.Conf.Lock.Kind {
	case "etcd":
		return etcd(ctx, identity)
	case "local":
		return flock(ctx, identity)
	}
	panic("unhandled lock kind " + global.Conf.Lock.Kind)
}
