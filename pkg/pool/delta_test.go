package pool_test

import (
	"testing"

	"github.com/ctfer-io/chall-manager/pkg/pool"
	"github.com/stretchr/testify/assert"
)

func Test_U_Delta(t *testing.T) {
	t.Parallel()

	var tests = map[string]struct {
		NewMin, NewMax, NumClaimed, NumPooled int64
		ExpectedDelta                         pool.Delta
	}{
		"min-decrease": {
			// Decrease from 3 to 1 pre-provisionned instances
			NewMin: 1,
			// Keep pooling with no limit
			NewMax: 0,
			// None has been claimed yet, and 3 are pooled due to OldMin=3
			NumClaimed: 0,
			NumPooled:  3,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 2, // min decreased by 2
			},
		},
		"min-increase": {
			// Increase from 1 to 3 pre-provisionned instances
			NewMin: 3,
			// Keep pooling with no limit
			NewMax: 0,
			// None has been claimed yet
			NumClaimed: 0,
			NumPooled:  1,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 2, // min increased by 2
				Delete: 0,
			},
		},
		"max-increase": {
			// Don't increase minimum instances pooled
			NewMin: 2,
			// Increase maximum number of instances
			NewMax: 6,
			// Let's say one has been claimed
			NumClaimed: 1,
			NumPooled:  2,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 0,
			},
		},
		"max-increase-new-room": {
			// Don't increase minimum instances pooled
			NewMin: 2,
			// Increase maximum number of instances
			NewMax: 6,
			// 3 were claimed, only 1 has been pooled because it reached max number of instances
			NumClaimed: 2,
			NumPooled:  1,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 1, // maximum increased enough for new pooled instances
				Delete: 0,
			},
		},
		"max-decrease": {
			// Don't increase minimum instances pooled
			NewMin: 2,
			// Decrease maximum number of instances
			NewMax: 2,
			// Let's say one has been claimed
			NumClaimed: 1,
			NumPooled:  2,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 1, // maximum decreased reducing the number of pool instances
			},
		},
		"max-decrease-enough-room": {
			// Don't increase minimum instances pooled
			NewMin: 2,
			// Decrease maximum number of instances
			NewMax: 4,
			// Let's say one has been claimed
			NumClaimed: 1,
			NumPooled:  2,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 0,
				Delete: 0,
			},
		},
		"pool-empty-until": {
			// Don't increase minimum instances pooled
			NewMin: 2,
			// Don't increase maximum number of instances
			NewMax: 4,
			// Let's say none has been claimed, but they were no
			// challenge in the pool due to challenge expiration.
			// Now it has changed, the delta must be re-computed and match
			// the minimal pool size.
			// Handling whether they should be created is left to the API
			// that has knownledge of the challenge until date.
			NumClaimed: 0,
			NumPooled:  0,
			// This leads to ...
			ExpectedDelta: pool.Delta{
				Create: 2,
				Delete: 0,
			},
		},
	}

	for testname, tt := range tests {
		t.Run(testname, func(t *testing.T) {
			d := pool.NewDelta(tt.NewMin, tt.NewMax, tt.NumClaimed, tt.NumPooled)
			assert.Equal(t, tt.ExpectedDelta, d)
		})
	}
}
