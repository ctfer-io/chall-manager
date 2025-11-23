package errors

import "errors"

// ErrChallengeExpired signals that a challenge cannot be instantiated anymore.
var ErrChallengeExpired = errors.New("challenge is already expired")

// ErrPoolExhausted signals that no more instances can be allocated (pool or max).
var ErrPoolExhausted = errors.New("no more instances available for this challenge")
