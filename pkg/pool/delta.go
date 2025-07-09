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
func NewDelta(newMin, newMax, numClaimed, numPooled int64) (d Delta) {
	desiredPool := computeDesiredPool(newMin, newMax, numClaimed)

	if desiredPool > numPooled {
		d.Create = desiredPool - numPooled
	} else if desiredPool < numPooled {
		d.Delete = numPooled - desiredPool
	}

	return
}

func computeDesiredPool(minVal, maxVal, claimed int64) int64 {
	if maxVal == 0 {
		return minVal
	}
	availablePool := maxVal - claimed
	if availablePool < 0 {
		availablePool = 0
	}
	if minVal < availablePool {
		return minVal
	}
	return availablePool
}
