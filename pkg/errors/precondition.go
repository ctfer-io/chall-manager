package errors

import "errors"

// ErrChallengeExpired signals that a challenge cannot be instantiated anymore.
var ErrChallengeExpired = errors.New("challenge is already expired")
