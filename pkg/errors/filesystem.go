package errors

import "fmt"

// ErrFilesystem wraps unexpected filesystem errors for clearer messaging.
type ErrFilesystem struct {
	Op  string
	Err error
}

func (e ErrFilesystem) Error() string {
	if e.Err == nil {
		return "filesystem error"
	}
	if e.Op != "" {
		return fmt.Sprintf("filesystem error during %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("filesystem error: %v", e.Err)
}
