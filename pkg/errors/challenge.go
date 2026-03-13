package errors

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChallengeExist struct {
	ID    string
	Exist bool
}

var _ error = (*ChallengeExist)(nil)

func (err *ChallengeExist) Error() string {
	return err.statusError().Error()
}

var _ meaningfulError = (*ChallengeExist)(nil)

func (err *ChallengeExist) statusError() error {
	if err.Exist {
		st, serr := status.New(codes.AlreadyExists, "Challenge already exists.").WithDetails(
			&errdetails.ErrorInfo{
				Reason: ReasonChallengeAlreadyExists,
				Domain: Domain,
				Metadata: map[string]string{
					"id": err.ID,
				},
			},
			&errdetails.ResourceInfo{
				ResourceType: "Challenge",
				ResourceName: err.ID,
				Description:  "A challenge with this ID already exists.",
			},
		)
		if serr != nil {
			return status.Errorf(codes.Internal, "failed to build error: %v", serr)
		}
		return st.Err()
	}

	st, serr := status.New(codes.NotFound, "Challenge not found.").WithDetails(
		&errdetails.ErrorInfo{
			Reason: "CHALLENGE_NOT_FOUND",
			Domain: Domain,
			Metadata: map[string]string{
				"id": err.ID,
			},
		},
		&errdetails.ResourceInfo{
			ResourceType: "Challenge",
			ResourceName: err.ID,
			Description:  "No challenge with this ID was found.",
		},
	)
	if serr != nil {
		return status.Errorf(codes.Internal, "failed to build error: %v", serr)
	}
	return st.Err()
}
