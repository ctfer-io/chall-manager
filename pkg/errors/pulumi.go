package errors

import "errors"

// ErrPulumiCanceled signals that a Pulumi operation was canceled (e.g., SIGINT).
var ErrPulumiCanceled = errors.New("provisioning canceled")
