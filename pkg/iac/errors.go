package iac

import "fmt"

// ErrOutputRequired is an error returned when the stack state did not
// export a required key.
type ErrOutputRequired struct {
	Key string
}

var _ error = (*ErrOutputRequired)(nil)

func (err ErrOutputRequired) Error() string {
	return fmt.Sprintf("state output %s is required, please update the Challenge Scenario to ensure export of it", err.Key)
}
