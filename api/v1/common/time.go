package common

import "time"

// ComputeUntil returns an instance until date based on a challenge until
// date and the challenge timeout.
//
// No until, no timeout: nil
// No until, timeout:    now+timeout
// until, no timeout:    until
// until, timeout:       min(now+timeout, until)
func ComputeUntil(until *time.Time, timeout *time.Duration) *time.Time {
	if until == nil {
		if timeout == nil {
			return nil
		}
		u := time.Now().Add(*timeout)
		return &u
	}
	if timeout == nil {
		return until
	}
	u := time.Now().Add(*timeout)
	if u.Before(*until) {
		return &u
	}
	return until
}
