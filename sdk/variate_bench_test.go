package sdk_test

import (
	"testing"

	"github.com/ctfer-io/chall-manager/sdk"
)

var (
	Variated string
)

func BenchmarkVariate(b *testing.B) {
	var variated string
	for i := 0; i < b.N; i++ {
		variated = sdk.Variate("a0b1c2d3", "This is my super example flag!!!")
	}
	Variated = variated
}
