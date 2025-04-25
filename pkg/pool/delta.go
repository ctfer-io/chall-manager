package pool

type Delta struct {
	// Create defines the number of instances to spin up in the pool.
	// Value is positive.
	Create int64

	// Delete defines the number of instances to shoot out of the pool.
	// Value is positive.
	Delete int64
}

// NewDelta computes the operations to be performed on a challenge update
// that resizes the pool.
//
// Expects all integers to be positive.
func NewDelta(oldMin, newMin, oldMax, newMax, numClaimed int64) (d Delta) {
	currentPool := computePool(oldMin, oldMax, numClaimed)
	desiredPool := computePool(newMin, newMax, numClaimed)

	if desiredPool > currentPool {
		d.Create = desiredPool - currentPool
	} else if desiredPool < currentPool {
		d.Delete = currentPool - desiredPool
	}
	return
}

func computePool(minVal, maxVal, claimed int64) int64 {
	if maxVal == 0 {
		return minVal
	}
	maxPool := maxVal - claimed
	if maxPool < 0 {
		return 0
	}
	if minVal < maxPool {
		return minVal
	}
	return maxPool
}
