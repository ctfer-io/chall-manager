package errors

import (
	"fmt"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type InstanceExist struct {
	ChallengeID string
	SourceID    string
	Exist       bool
}

var _ error = (*InstanceExist)(nil)

func (err InstanceExist) Error() string {
	return err.statusError().Error()
}

var _ meaningfulError = (*InstanceExist)(nil)

func (err InstanceExist) statusError() error {
	if err.Exist {
		st, serr := status.New(codes.AlreadyExists, "Instance already exists.").WithDetails(
			&errdetails.ErrorInfo{
				Reason: ReasonInstanceAlreadyExists,
				Domain: Domain,
				Metadata: map[string]string{
					"challenge_id": err.ChallengeID,
					"source_id":    err.SourceID,
				},
			},
			&errdetails.ResourceInfo{
				ResourceType: "Instance",
				ResourceName: fmt.Sprintf("%s/%s", err.ChallengeID, err.SourceID),
				Description:  "An instance with this ID already exists.",
			},
		)
		if serr != nil {
			return status.Errorf(codes.Internal, "failed to build error: %v", serr)
		}
		return st.Err()
	}

	st, serr := status.New(codes.NotFound, "Instance not found.").WithDetails(
		&errdetails.ErrorInfo{
			Reason: ReasonInstanceNotFound,
			Domain: Domain,
			Metadata: map[string]string{
				"challenge_id": err.ChallengeID,
				"source_id":    err.SourceID,
			},
		},
		&errdetails.ResourceInfo{
			ResourceType: "Instance",
			ResourceName: fmt.Sprintf("%s/%s", err.ChallengeID, err.SourceID),
			Description:  "No instance with this ID was found.",
		},
	)
	if serr != nil {
		return status.Errorf(codes.Internal, "failed to build error: %v", serr)
	}
	return st.Err()
}
