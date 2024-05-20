package scenario

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// Validate check the challenge instance can build i.e. a preview.
func Validate(ctx context.Context, dir string) error {
	rand := randName()
	stack, err := iac.LoadStack(ctx, dir, rand)
	if err != nil {
		return err
	}
	if err := stack.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{
			Value: rand,
		},
	}); err != nil {
		return err
	}

	// Preview stack to ensure it build without error
	if _, err := stack.Preview(ctx); err != nil {
		return errs.ErrScenario
	}

	return nil
}

func randName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
