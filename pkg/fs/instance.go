package fs

import (
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

const (
	InstanceSubdir = "instance"
)

// Instance is the internal model of an API Instance as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/<id>/instance/<id>/info.json`)
type Instance struct {
	ChallengeID    string     `json:"challenge_id"`
	SourceID       string     `json:"source_id"`
	State          any        `json:"state"`
	Since          time.Time  `json:"since"`
	LastRenew      time.Time  `json:"last_renew"`
	Until          *time.Time `json:"until,omitempty"`
	ConnectionInfo string     `json:"connection_info"`
	Flag           *string    `json:"flag,omitempty"`
}

func InstanceDirectory(challId, sourceId string) string {
	return filepath.Join(ChallengeDirectory(challId), InstanceSubdir, Hash(sourceId))
}

func IdOfInstance(challId, idh string) (string, error) {
	f, err := os.Open(filepath.Join(global.Conf.Directory, "chall", Hash(challId), InstanceSubdir, idh, "info.json"))
	if err != nil {
		return "", &errs.ErrInternal{Sub: err}
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fsist := &Instance{}
	if err := dec.Decode(fsist); err != nil {
		return "", &errs.ErrInternal{Sub: err}
	}
	return fsist.SourceID, nil
}

// CheckInstance returns an error if there is no instance with the given ids.
func CheckInstance(challId, sourceId string) error {
	fpath := filepath.Join(InstanceDirectory(challId, sourceId), "info.json")
	if _, err := os.Stat(fpath); err != nil {
		return &errs.ErrInstanceExist{
			ChallengeID: challId,
			SourceID:    sourceId,
			Exist:       false,
		}
	}
	return nil
}

func LoadInstance(challId, sourceId string) (*Instance, error) {
	if err := CheckInstance(challId, sourceId); err != nil {
		return nil, err
	}

	fpath := filepath.Join(InstanceDirectory(challId, sourceId), "info.json")
	f, err := os.Open(fpath)
	if err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fsist := &Instance{}
	if err := dec.Decode(fsist); err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	return fsist, nil
}

func (ist *Instance) Save() error {
	idir := InstanceDirectory(ist.ChallengeID, ist.SourceID)
	_ = os.Mkdir(idir, os.ModePerm)

	fpath := filepath.Join(idir, "info.json")
	f, err := os.Create(fpath)
	if err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	defer fclose(f)

	enc := json.NewEncoder(f)
	if err := enc.Encode(ist); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}

func (ist *Instance) Delete() error {
	idir := InstanceDirectory(ist.ChallengeID, ist.SourceID)
	if err := os.Remove(idir); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}
