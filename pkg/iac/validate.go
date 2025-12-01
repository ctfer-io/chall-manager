package iac

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	fsapi "github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// Validate check the challenge scenario can preview without error (a basic check).
func Validate(ctx context.Context, fschall *fsapi.Challenge) error {
	// Track span of loading stack
	ctx, span := global.Tracer.Start(ctx, "validating-scenario")
	defer span.End()

	rand := randName()
	stack, err := LoadStack(ctx, fschall.Scenario, rand)
	if err != nil {
		return err
	}
	if err := stack.pas.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{
			Value: rand,
		},
	}); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	if err := Additional(ctx, stack, fschall.Additional, nil); err != nil {
		return &errs.ErrInternal{Sub: err}
	}

	// Preview stack to ensure it build without error
	if _, err := stack.pas.Preview(ctx); err != nil {
		return &errs.ErrScenario{Sub: err}
	}

	return nil
}

func randName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
