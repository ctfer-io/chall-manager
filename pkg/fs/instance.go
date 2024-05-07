package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// Instance is the internal model of an API Instance as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/<id>/instance/<id>/info.json`)
type Instance struct {
	ChallengeID    string    `json:"challenge_id"`
	SourceID       string    `json:"source_id"`
	State          any       `json:"state"`
	Since          time.Time `json:"since"`
	LastRenew      time.Time `json:"last_renew"`
	Until          time.Time `json:"until"`
	ConnectionInfo string    `json:"connection_info"`
	Flag           *string   `json:"flag,omitempty"`
}

func LoadInstance(challId, sourceId string) (*Instance, error) {
	challDir := filepath.Join(global.Conf.Directory, "chall", challId)
	idir := filepath.Join(challDir, "instance", sourceId)
	fpath := filepath.Join(idir, "info.json")
	if _, err := os.Stat(fpath); err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	f, err := os.ReadFile(fpath)
	if err != nil {
		return nil, &errs.ErrInstanceExist{
			ChallengeID: challId,
			SourceID:    sourceId,
			Exist:       false,
		}
	}
	fsist := &Instance{}
	if err := json.Unmarshal(f, fsist); err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	return fsist, nil
}

func (ist *Instance) Save() error {
	challDir := filepath.Join(global.Conf.Directory, "chall", ist.ChallengeID)
	idir := filepath.Join(challDir, "instance", ist.SourceID)
	fsb, err := json.Marshal(ist)
	if err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	if err := os.WriteFile(filepath.Join(idir, "info.json"), fsb, 0644); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}
