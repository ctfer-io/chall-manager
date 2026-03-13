package common

import (
	"context"
	"errors"
	"net/url"

	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/iac"
)

// Validate a scenario given its OCI reference (and additional k=v map for variability).
// It processes errors for meaningfull error codes, as it is the entrypoint toward downstream services.
func Validate(ctx context.Context, ref string, add map[string]string) error {
	err := iac.Validate(ctx, ref, add)
	if err == nil {
		return nil
	}

	logger := global.Log()

	switch err := err.(type) {
	case *errs.OCIInteraction:
		// If we can get more precise errors from sub-error, do it.
		// Else return a more generic error but log the full one for troubleshooting purposes.

		rp, _ := registry.ParseReference(ref)

		// Network failure
		if sub, ok := err.Sub.(*url.Error); ok {
			logger.Debug(ctx, "registry is unreachable",
				zap.Error(sub),
				zap.String("reference", err.Ref),
				zap.String("registry", rp.Registry),
			)

			st, serr := status.New(
				codes.FailedPrecondition, // Might be a transient network error, could retry later once fixed
				"Registry is not available. You might want to try again later.",
			).WithDetails(
				&errdetails.ErrorInfo{
					Reason: errs.ReasonOCIInteraction,
					Domain: errs.Domain + "/OCI",
					Metadata: map[string]string{
						"reference": err.Ref,
					},
				},
				&errdetails.BadRequest{
					FieldViolations: []*errdetails.BadRequest_FieldViolation{
						{
							Field:       "scenario",
							Reason:      errs.ReasonOCIInteraction,
							Description: "Registry is unreachable.",
						},
					},
				},
			)
			if serr != nil {
				return status.Newf(codes.Internal, "failed to build error: %v", serr).Err()
			}
			return st.Err()
		}

		// Other failures
		sub := errors.Unwrap(err.Sub)
		switch {
		case errors.Is(sub, errdef.ErrNotFound):
			// The manifest/repo is not found

			logger.Debug(ctx, "reference not found",
				zap.Error(sub),
				zap.String("reference", err.Ref),
				zap.String("registry", rp.Registry),
			)

			st, serr := status.New(
				codes.InvalidArgument, // Not our problem that this reference does not exist
				"Registry does not contain reference.",
			).WithDetails(
				&errdetails.ErrorInfo{
					Reason: errs.ReasonOCINotFound,
					Domain: errs.Domain + "/OCI",
					Metadata: map[string]string{
						"reference": err.Ref,
					},
				},
				&errdetails.BadRequest{
					FieldViolations: []*errdetails.BadRequest_FieldViolation{
						{
							Field:       "scenario",
							Reason:      errs.ReasonOCINotFound,
							Description: "Reference not found.",
						},
					},
				},
			)
			if serr != nil {
				return status.Newf(codes.Internal, "failed to build error: %v", serr).Err()
			}
			return st.Err()

		case errors.Is(sub, auth.ErrBasicCredentialNotFound):
			// The registry has authentication, no valid credentials provided

			logger.Debug(ctx, "authentication failed to registry",
				zap.Error(sub),
				zap.String("reference", err.Ref),
				zap.String("registry", rp.Registry),
			)

			st, serr := status.New(
				codes.FailedPrecondition,
				"Registry requires authentication. You may not be authorized to use it.",
			).WithDetails(
				&errdetails.ErrorInfo{
					Reason: errs.ReasonOCIInteraction,
					Domain: errs.Domain + "/OCI",
					Metadata: map[string]string{
						"reference": err.Ref,
					},
				},
				&errdetails.BadRequest{
					FieldViolations: []*errdetails.BadRequest_FieldViolation{
						{
							Field: "scenario",
							Description: "The reference of provided scenario is hosted on a registry that requires authentication, " +
								"for which credentials did not match.",
							Reason: errs.ReasonOCIInteraction,
						},
					},
				},
			)
			if serr != nil {
				return status.Newf(codes.Internal, "failed to build error: %v", serr).Err()
			}
			return st.Err()
		}

		logger.Error(ctx, "interacting with OCI registry",
			zap.String("reference", err.Ref),
			zap.Error(err.Sub),
		)
		return err

	case *errs.Scenario:
		err.FieldViolation = &errdetails.BadRequest_FieldViolation{
			Field:       "scenario",
			Reason:      "VALIDATION",
			Description: "", // will be filled by the meaningful error itself
		}

	default: // *errs.ErrPreprocess or Internal Server Error
		logger.Error(ctx, "validating scenario",
			zap.String("reference", ref),
			zap.Error(err),
		)
	}

	return err
}
