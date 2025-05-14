package global

import (
	"context"
	"time"
)

type withoutCtx struct {
	parent  context.Context
	without any
}

var _ context.Context = (*withoutCtx)(nil)

func (withoutCtx) Deadline() (deadline time.Time, ok bool) {
	return
}

func (withoutCtx) Done() <-chan struct{} {
	return nil
}

func (withoutCtx) Err() error {
	return nil
}

func (c withoutCtx) Value(key any) any {
	if key == c.without {
		return nil
	}
	return c.parent.Value(key)
}

func WithChallengeID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, challengeKey{}, id)
}

func WithIdentity(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func WithSourceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sourceKey{}, id)
}

func WithoutChallengeID(ctx context.Context) context.Context {
	return &withoutCtx{
		parent:  ctx,
		without: challengeKey{},
	}
}

func WithoutIdentity(ctx context.Context) context.Context {
	return &withoutCtx{
		parent:  ctx,
		without: identityKey{},
	}
}

func WithoutSourceID(ctx context.Context) context.Context {
	return &withoutCtx{
		parent:  ctx,
		without: sourceKey{},
	}
}
