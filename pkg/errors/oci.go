package errors

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MalformedOCIReference struct {
	Ref string
	Sub error
}

var _ error = (*MalformedOCIReference)(nil)

func (err MalformedOCIReference) Error() string {
	return err.statusError().Error()
}

var _ meaningfulError = (*MalformedOCIReference)(nil)

func (err MalformedOCIReference) statusError() error {
	st, serr := status.New(codes.InvalidArgument, "OCI reference is malformed.").WithDetails(
		&errdetails.ErrorInfo{
			Reason: ReasonOCIMalformedRef,
			Domain: Domain,
			Metadata: map[string]string{
				"reference": err.Ref,
			},
		},
		&errdetails.ResourceInfo{
			ResourceType: "Reference",
			ResourceName: err.Ref,
			Description:  err.Sub.Error(),
		},
		&errdetails.Help{
			Links: []*errdetails.Help_Link{
				{
					Description: "OCI Image Format specification GitHub repository.",
					Url:         "https://github.com/opencontainers/image-spec",
				},
			},
		},
	)
	if serr != nil {
		return status.Errorf(codes.Internal, "failed to build error: %v", serr)
	}
	return st.Err()
}

type OCIInteraction struct {
	Ref string
	Sub error
}

var _ error = (*OCIInteraction)(nil)

func (err OCIInteraction) Error() string {
	return err.statusError().Error()
}

var _ meaningfulError = (*OCIInteraction)(nil)

func (err OCIInteraction) statusError() error {
	st, serr := status.New(codes.Internal, "OCI interaction failed.").WithDetails(
		&errdetails.ErrorInfo{
			Reason: ReasonOCIInteraction,
			Domain: Domain,
			Metadata: map[string]string{
				"reference": err.Ref,
			},
		},
	)
	if serr != nil {
		return status.Errorf(codes.Internal, "failed to build error: %v", serr)
	}
	return st.Err()
}
