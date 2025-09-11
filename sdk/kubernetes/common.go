package kubernetes

import (
	"crypto/sha1"
	"encoding/hex"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	defaultCIDR = "0.0.0.0/0"
)

type DepCon struct {
	name string
	deps []string
}

var _ Resource = (*DepCon)(nil)

func (d *DepCon) GetID() string             { return d.name }
func (d *DepCon) GetDependencies() []string { return d.deps }

// randName is a pseudo-random name generator. It does not include
// random under the hood thus is reproducible.
func randName(seed string) string {
	h := sha1.New()
	if _, err := h.Write([]byte(seed)); err != nil {
		// This will happen only if FIPS compliance is turned on
		panic(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ptr[T any](t T) *T {
	return &t
}

func defaults[T comparable](v any, def T) T {
	if v == nil {
		return def
	}
	if nv, ok := v.(T); ok {
		if nv == *new(T) {
			return def
		}
		return nv
	}
	panic("invalid setup")
}

func merge(all ...pulumi.StringMapOutput) pulumi.StringMapOutput {
	if len(all) == 0 {
		return pulumi.StringMap{}.ToStringMapOutput()
	}
	out := all[0]
	for _, b := range all[1:] {
		out = pulumi.All(out, b).ApplyT(func(all []any) map[string]string {
			o := all[0].(map[string]string)
			no := all[1].(map[string]string)
			for k, v := range no {
				o[k] = v
			}
			return o
		}).(pulumi.StringMapOutput)
	}
	return out
}

func raw(o pulumi.StringOutput) string {
	var s string
	wg := sync.WaitGroup{}
	wg.Add(1)
	o.ApplyT(func(v string) error {
		s = v
		wg.Done()
		return nil
	})
	wg.Wait()
	return s
}

func lenP(o pulumi.StringArrayOutput) int {
	var l int
	wg := sync.WaitGroup{}
	wg.Add(1)
	o.ApplyT(func(v []string) error {
		l = len(v)
		wg.Done()
		return nil
	})
	wg.Wait()
	return l
}
