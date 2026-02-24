package errors

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
)

type Scenario struct {
	Ref string
	Sub error

	// FieldViolation is an optional error detail for API layers to provide clearer messages to downstream consumers
	FieldViolation *errdetails.BadRequest_FieldViolation
}

var _ error = (*Scenario)(nil)

func (err Scenario) Error() string {
	return err.statusError().Error()
}

var _ meaningfulError = (*Scenario)(nil)

func (err Scenario) statusError() error {
	details := []protoadapt.MessageV1{
		&errdetails.ErrorInfo{
			Reason: ReasonScenarioNonMatchingSpec,
			Domain: Domain,
			Metadata: map[string]string{
				"reference": err.Ref,
			},
		},
		&errdetails.ResourceInfo{
			ResourceType: "Scenario",
			ResourceName: err.Ref,
			Description:  "A Pulumi program acting as a challenge instance deployment scenario.",
		},
		&errdetails.Help{
			Links: []*errdetails.Help_Link{
				{
					Description: "Definition of a Scenario.",
					Url:         "https://ctfer.io/docs/chall-manager/glossary/#scenario",
				},
				{
					Description: "Step-by-step guide to create a Scenario.",
					Url:         "https://ctfer.io/docs/chall-manager/challmaker-guides/create-scenario/",
				},
			},
		},
	}
	if err.FieldViolation != nil {
		err.FieldViolation.Description = err.Sub.Error()
		details = append(details,
			&errdetails.BadRequest{
				FieldViolations: []*errdetails.BadRequest_FieldViolation{err.FieldViolation},
			},
		)
	}

	st, serr := status.New(codes.InvalidArgument, "Scenario does not match specification.").WithDetails(details...)
	if serr != nil {
		return status.Errorf(codes.Internal, "failed to build error: %v", serr)
	}
	return st.Err()
}

type Preprocess struct {
	Dir, Ref string
	Sub      error
}

var _ error = (*Preprocess)(nil)

func (err Preprocess) Error() string {
	return err.statusError().Error()
}

var _ meaningfulError = (*Preprocess)(nil)

func (err Preprocess) statusError() error {
	st, serr := status.New(codes.Internal, "Scenario pre-processing failed.").WithDetails(
		&errdetails.ErrorInfo{
			Reason: ReasonScenarioPreprocess,
			Domain: Domain,
			Metadata: map[string]string{
				"directory": err.Dir,
			},
		},
		&errdetails.ResourceInfo{
			ResourceType: "Scenario",
			ResourceName: err.Ref,
			Description:  err.Sub.Error(),
		},
	)
	if serr != nil {
		return status.Errorf(codes.Internal, "failed to build error: %v", serr)
	}
	return st.Err()
}
