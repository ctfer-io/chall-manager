package errors

import "errors"

// ErrRenewNotAllowed signals that the challenge does not accept renewals.
var ErrRenewNotAllowed = errors.New("challenge does not accept renewal")

// ErrInstanceExpiredRenew signals that the instance is already expired, so it cannot be renewed.
var ErrInstanceExpiredRenew = errors.New("challenge instance can't be renewed as it expired")
