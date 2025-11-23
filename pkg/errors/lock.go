package errors

import "errors"

// ErrLockUnavailable signals that the requested resource is currently locked or lock acquisition failed.
var ErrLockUnavailable = errors.New("resource is locked, try again")
