package sdk_test

import (
	"testing"

	"github.com/ctfer-io/chall-manager/sdk"
)

func FuzzVariate(f *testing.F) {
	f.Fuzz(func(t *testing.T, identity, base string, low, up, num, spec bool) {
		_ = sdk.Variate(identity, base,
			sdk.WithLowercase(low),
			sdk.WithUppercase(up),
			sdk.WithNumeric(num),
			sdk.WithSpecial(spec),
		)
	})
}
