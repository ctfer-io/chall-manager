package errors

import "github.com/pkg/errors"

type ErrInternal struct {
	Sub error
}

func (err ErrInternal) Error() string {
	// If embedded internal server error, unwrap it
	if err, ok := err.Sub.(*ErrInternal); ok {
		return err.Sub.Error()
	}
	return errors.Wrap(err.Sub, ErrInternalNoSub.Error()).Error()
}

var (
	ErrInternalNoSub = errors.New("internal server error")
)
