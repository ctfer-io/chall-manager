package iac

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Validate check the challenge scenario can preview without error (a basic check).
func Validate(ctx context.Context, ref string, additional map[string]string) error {
	ctx, span := global.Tracer.Start(ctx, "scenario.validation", trace.WithAttributes(
		attribute.String("reference", ref),
	))
	defer span.End()

	rand := randName()
	stack, err := LoadStack(ctx, ref, rand)
	if err != nil {
		return err
	}
	if err := stack.pas.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{
			Value: rand,
		},
	}); err != nil {
		return &errs.Scenario{
			Ref: ref,
			Sub: err,
		}
	}
	if err := Additional(ctx, stack, additional, nil); err != nil {
		return &errs.Scenario{
			Ref: ref,
			Sub: err,
		}
	}

	// Preview stack to ensure it build without error
	if _, err := stack.pas.Preview(ctx); err != nil {
		return &errs.Scenario{
			Ref: ref,
			Sub: err,
		}
	}

	return nil
}

func randName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
