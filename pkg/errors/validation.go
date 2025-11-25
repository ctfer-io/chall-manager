package errors

// ErrValidationFailed signals that a request failed validation.
type ErrValidationFailed struct {
	Reason string
}

func (err ErrValidationFailed) Error() string {
	if err.Reason == "" {
		return "validation failed"
	}
	return err.Reason
}
