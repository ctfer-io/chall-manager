package errors

import (
	"fmt"

	"github.com/pkg/errors"
)

type ErrScenario struct {
	Sub error
}

var _ error = (*ErrScenario)(nil)

func (err ErrScenario) Error() string {
	return fmt.Sprintf("invalid scenario: %s", err.Sub)
}

var (
	ErrScenarioNoSub = errors.New("invalid scenario")
)
