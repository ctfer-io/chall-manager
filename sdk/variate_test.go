package sdk_test

import (
	"testing"

	"github.com/ctfer-io/chall-manager/sdk"
	"github.com/stretchr/testify/assert"
)

func Test_U_Variate(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	const identity = "a0b1c2d3"
	const base = "This is my super example flag!!!"

	// Following test case asserts default variations, plus special
	// character are not variated (still in default configuration).
	variated := sdk.Variate(identity, base)
	assert.NotEqual(base, variated)
	assert.Equal(variated[len(variated)-3:], "!!!")

	// Following test case asserts reproducibility of the variation.
	// This should not be happening in production, but guarantees
	// stability of operations if they does not complete for whatever
	// reason and retry.
	if !assert.Equal(variated, sdk.Variate(identity, base)) {
		return // following test case depends on it, don't even try
	}

	// Following test case asserts default configuration if to not
	// variated over special characters, mostly for readability.
	assert.Equal(variated, sdk.Variate(identity, base,
		sdk.WithLowercase(true),
		sdk.WithUppercase(true),
		sdk.WithNumeric(true),
		sdk.WithSpecial(false),
	))

	// Following test case asserts custom (special=true) variations
	// propagates to special characters.
	variated = sdk.Variate(identity, base, sdk.WithSpecial(true))
	assert.NotEqual(base, variated)
	assert.NotEqual(variated[len(variated)-3:], "!!!")

	// Following test case asserts non-pritable ascii extended character
	// is not variated.
	// For this purpose, we encourage one who require it to use another
	// leet implementation.
	nonAE := string([]byte{0x00})
	variated = sdk.Variate(identity, nonAE)
	assert.Equal(nonAE, variated)

	// Following test cases asserts non-8-long identity can also work.
	// This should not happen, but even if it does it should not crash.
	sdk.Variate("0123", base)
	sdk.Variate("0123456789abcdef", base)
}
