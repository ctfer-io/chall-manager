package server

import (
	"testing"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusFromError_InstanceExists(t *testing.T) {
	err := statusFromError(&errs.ErrInstanceExist{
		ChallengeID: "1",
		SourceID:    "2",
		Exist:       true,
	})

	st := status.Convert(err)
	if st.Code() != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %s", st.Code())
	}
}

func TestStatusFromError_InstanceMissing(t *testing.T) {
	err := statusFromError(&errs.ErrInstanceExist{
		ChallengeID: "1",
		SourceID:    "2",
		Exist:       false,
	})

	st := status.Convert(err)
	if st.Code() != codes.NotFound {
		t.Fatalf("expected NotFound, got %s", st.Code())
	}
}

func TestStatusFromError_ExpiredChallenge(t *testing.T) {
	err := statusFromError(errs.ErrChallengeExpired)
	st := status.Convert(err)
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %s", st.Code())
	}
}

func TestStatusFromError_PoolExhausted(t *testing.T) {
	err := statusFromError(errs.ErrPoolExhausted)
	st := status.Convert(err)
	if st.Code() != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted, got %s", st.Code())
	}
}

func TestStatusFromError_ProvisioningFailed(t *testing.T) {
	err := statusFromError(errs.ErrProvisioningFailed{})
	st := status.Convert(err)
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %s", st.Code())
	}
}

func TestStatusFromError_LockUnavailable(t *testing.T) {
	err := statusFromError(errs.ErrLockUnavailable)
	st := status.Convert(err)
	if st.Code() != codes.Aborted {
		t.Fatalf("expected Aborted, got %s", st.Code())
	}
}

func TestStatusFromError_RenewNotAllowed(t *testing.T) {
	err := statusFromError(errs.ErrRenewNotAllowed)
	st := status.Convert(err)
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %s", st.Code())
	}
}

func TestStatusFromError_RenewExpired(t *testing.T) {
	err := statusFromError(errs.ErrInstanceExpiredRenew)
	st := status.Convert(err)
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %s", st.Code())
	}
}

func TestStatusFromError_ValidationFailed(t *testing.T) {
	err := statusFromError(errs.ErrValidationFailed{Reason: "bad input"})
	st := status.Convert(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %s", st.Code())
	}
}
