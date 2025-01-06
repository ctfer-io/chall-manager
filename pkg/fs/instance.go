package fs

import (
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"
	"go.uber.org/multierr"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// Instance is the internal model of an API Instance as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/<id>/instance/<id>/info.json`)
type Instance struct {
	Identity       string     `json:"identity"`
	ChallengeID    string     `json:"challenge_id"`
	SourceID       string     `json:"source_id"`
	State          any        `json:"state"`
	Since          time.Time  `json:"since"`
	LastRenew      time.Time  `json:"last_renew"`
	Until          *time.Time `json:"until,omitempty"`
	ConnectionInfo string     `json:"connection_info"`
	Flag           *string    `json:"flag,omitempty"`
}

func InstanceDirectory(challID, sourceID string) string {
	return filepath.Join(ChallengeDirectory(challID), instanceSubdir, Hash(sourceID))
}

// CheckInstance returns an error if there is no instance with the given ids.
func CheckInstance(challID, sourceID string) error {
	fpath := filepath.Join(InstanceDirectory(challID, sourceID), infoFile)
	if _, err := os.Stat(fpath); err != nil {
		return &errs.ErrInstanceExist{
			ChallengeID: challID,
			SourceID:    sourceID,
			Exist:       false,
		}
	}
	return nil
}

func ListInstances(challID string) (iids []string, merr error) {
	challDir := ChallengeDirectory(challID)
	dir, err := os.ReadDir(filepath.Join(challDir, instanceSubdir))
	if err != nil {
		return
	}
	for _, dfs := range dir {
		iid, err := idOfInstance(challID, dfs.Name())
		if err != nil {
			merr = multierr.Append(merr, err)
		}
		iids = append(iids, iid)
	}
	if merr != nil {
		return nil, merr
	}
	return
}

func LoadInstance(challID, sourceID string) (*Instance, error) {
	if err := CheckInstance(challID, sourceID); err != nil {
		return nil, err
	}

	fpath := filepath.Join(InstanceDirectory(challID, sourceID), infoFile)
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

	fpath := filepath.Join(idir, infoFile)
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
	if err := os.RemoveAll(idir); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}

func idOfInstance(challID, idh string) (string, error) {
	f, err := os.Open(filepath.Join(ChallengeDirectory(challID), instanceSubdir, idh, infoFile))
	if err != nil {
		return "", err
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fsist := &Instance{}
	if err := dec.Decode(fsist); err != nil {
		return "", err
	}
	return fsist.SourceID, nil
}
