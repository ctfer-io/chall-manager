package pool_test

import (
	"testing"

	"github.com/ctfer-io/chall-manager/pkg/pool"
	"github.com/stretchr/testify/assert"
)

func Test_U_Delta(t *testing.T) {
	t.Parallel()

	var tests = map[string]struct {
		OldMin, NewMin, OldMax, NewMax, NumClaimed int64
		ExpectedDelta                              pool.Delta
	}{
		"min-decrease": {
			// Decrease from 3 to 1 pre-provisionned instances
			OldMin: 3,
			NewMin: 1,
			// Keep pooling with no limit
			OldMax: 0,
			NewMax: 0,
			// None has been claimed yet, and 3 are pooled due to OldMin=3
			NumClaimed: 0,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 2, // min decreased by 2
			},
		},
		"min-increase": {
			// Increase from 1 to 3 pre-provisionned instances
			OldMin: 1,
			NewMin: 3,
			// Keep pooling with no limit
			OldMax: 0,
			NewMax: 0,
			// None has been claimed yet
			NumClaimed: 0,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 2, // min increased by 2
				Delete: 0,
			},
		},
		"max-increase": {
			// Don't increase minimum instances pooled
			OldMin: 2,
			NewMin: 2,
			// Increase maximum number of instances
			OldMax: 4,
			NewMax: 6,
			// Let's say one has been claimed
			NumClaimed: 1,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 0,
			},
		},
		"max-increase-new-room": {
			// Don't increase minimum instances pooled
			OldMin: 2,
			NewMin: 2,
			// Increase maximum number of instances
			OldMax: 3,
			NewMax: 6,
			// 3 were claimed, only 1 has been pooled because it reached max number of instances
			NumClaimed: 2,
			ExpectedDelta: pool.Delta{
				Create: 1, // maximum increased enough for new pooled instances
				Delete: 0,
			},
		},
		"max-decrease": {
			// Don't increase minimum instances pooled
			OldMin: 2,
			NewMin: 2,
			// Decrease maximum number of instances
			OldMax: 4,
			NewMax: 2,
			// Let's say one has been claimed
			NumClaimed: 1,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 1, // maximum decreased reducing the number of pool instances
			},
		},
		"max-decrease-enough-room": {
			// Don't increase minimum instances pooled
			OldMin: 2,
			NewMin: 2,
			// Decrease maximum number of instances
			OldMax: 6,
			NewMax: 4,
			// Let's say one has been claimed
			NumClaimed: 1,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 0,
			},
		},
	}

	for testname, tt := range tests {
		t.Run(testname, func(t *testing.T) {
			d := pool.NewDelta(tt.OldMin, tt.NewMin, tt.OldMax, tt.NewMax, tt.NumClaimed)
			assert.Equal(t, tt.ExpectedDelta, d)
		})
	}
}
