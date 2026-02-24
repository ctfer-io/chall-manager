package common

import (
	"fmt"
	"slices"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// CheckPooler looks into update mask paths if the pooler bondaries are coherent.
// If incoherent, returns a non-nil error the business layer can return.
func CheckPooler(paths []string, min, max int64) error {
	fv := []*errdetails.BadRequest_FieldViolation{}
	if slices.Contains(paths, "min") && min < 0 {
		fv = append(fv, &errdetails.BadRequest_FieldViolation{
			Field:       "min",
			Reason:      "MUST_BE_POSITIVE",
			Description: "Minimum boundary must be a positive integer.",
		})
	}
	if slices.Contains(paths, "max") && max < 0 {
		fv = append(fv, &errdetails.BadRequest_FieldViolation{
			Field:       "max",
			Reason:      "MUST_BE_POSITIVE",
			Description: "Maximum boundary must be a positive integer.",
		})
	}
	if slices.Contains(paths, "min") && slices.Contains(paths, "max") && max > 0 && min > max {
		fv = append(fv, &errdetails.BadRequest_FieldViolation{
			Field:       "min",
			Reason:      "INVERTED_BOUNDARIES",
			Description: "When an upper bound is defined, minimum cannot exceed maximum.",
		})
	}
	if len(fv) == 0 {
		return nil
	}

	metadata := map[string]string{}
	if slices.Contains(paths, "min") {
		metadata["min"] = fmt.Sprintf("%d", min)
	}
	if slices.Contains(paths, "max") {
		metadata["max"] = fmt.Sprintf("%d", max)
	}

	st, err := status.New(codes.InvalidArgument, "Pooler configuration out of bounds.").WithDetails(
		&errdetails.ErrorInfo{
			Reason:   errs.ReasonChallengePoolerOOB,
			Domain:   errs.Domain,
			Metadata: metadata,
		},
		&errdetails.BadRequest{
			FieldViolations: fv,
		},
	)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to build error: %v", err)
	}
	return st.Err()
}

func CheckUpdateMask(fm *fieldmaskpb.FieldMask, m proto.Message) error {
	if fm == nil || fm.IsValid(m) {
		return nil
	}
	st, err := status.New(codes.InvalidArgument, "Invalid update mask.").WithDetails(
		&errdetails.ErrorInfo{
			Reason: errs.ReasonChallengeInvalidUM,
			Domain: errs.Domain,
			Metadata: map[string]string{
				"update-mask": fm.String(),
			},
		},
		&errdetails.BadRequest{
			FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{
					Field:       "update-mask",
					Reason:      "INVALID_UPDATE_MASK",
					Description: "The update mask is malformed.",
				},
			},
		},
		&errdetails.Help{
			Links: []*errdetails.Help_Link{
				{
					Description: "Protocol Buffers > Well-Known Types > Field mask",
					Url:         "https://protobuf.dev/reference/protobuf/google.protobuf/#field-mask",
				},
				{
					Description: "AIP-161 Field masks",
					Url:         "https://google.aip.dev/161",
				},
			},
		},
	)
	if err != nil {
		return status.Newf(codes.Internal, "failed to build error: %v", err).Err()
	}
	return st.Err()
}
