package errors

import "fmt"

// ErrProvisioningFailed wraps a failure during infrastructure provisioning.
type ErrProvisioningFailed struct {
	Sub error
}

func (err ErrProvisioningFailed) Error() string {
	if err.Sub == nil {
		return "provisioning failed"
	}
	return fmt.Sprintf("provisioning failed: %v", err.Sub)
}
