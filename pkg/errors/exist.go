package errors

import "fmt"

type ErrChallengeExist struct {
	ID    string
	Exist bool
}

func (err ErrChallengeExist) Error() string {
	if err.Exist {
		return fmt.Sprintf("challenge %s already exist", err.ID)
	}
	return fmt.Sprintf("challenge %s does not exist", err.ID)
}

type ErrInstanceExist struct {
	ChallengeID string
	SourceID    string
	Exist       bool
}

func (err ErrInstanceExist) Error() string {
	if err.Exist {
		return fmt.Sprintf("instance of challenge %s and identity %s already exist", err.ChallengeID, err.SourceID)
	}
	return fmt.Sprintf("instance of challenge %s and identity %s does not exist", err.ChallengeID, err.SourceID)
}
