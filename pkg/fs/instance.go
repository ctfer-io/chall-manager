package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// Instance is the internal model of an API Instance as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/<id>/instance/<id>/info.json`)
type Instance struct {
	Identity       string            `json:"identity"`
	ChallengeID    string            `json:"challenge_id"`
	State          any               `json:"state"`
	Since          time.Time         `json:"since"`
	LastRenew      time.Time         `json:"last_renew"`
	Until          *time.Time        `json:"until,omitempty"`
	ConnectionInfo string            `json:"connection_info"`
	Flag           *string           `json:"flag,omitempty"`
	Additional     map[string]string `json:"additional,omitempty"`
}

func Claim(challID, identity, sourceID string) error {
	fsist := &Instance{
		ChallengeID: challID,
		Identity:    identity,
	}
	return fsist.Claim(sourceID)
}

func (ist *Instance) Claim(sourceID string) error {
	claimPath := filepath.Join(InstanceDirectory(ist.ChallengeID, ist.Identity), "claim")
	if _, err := os.Stat(claimPath); err == nil {
		return fmt.Errorf("instance %s/%s is already claimed", ist.ChallengeID, ist.Identity)
	}
	return os.WriteFile(claimPath, []byte(sourceID), 0600)
}

func (ist *Instance) IsClaimed() bool {
	claimPath := filepath.Join(InstanceDirectory(ist.ChallengeID, ist.Identity), "claim")
	_, err := os.Stat(claimPath)
	return err == nil
}

func LookupClaim(challID, identity string) (string, error) {
	claimPath := filepath.Join(InstanceDirectory(challID, identity), "claim")
	b, err := os.ReadFile(claimPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func FindInstance(challID, sourceID string) (string, error) {
	ists, err := ListInstances(challID)
	if err != nil {
		return "", err
	}
	for _, ist := range ists {
		src, err := LookupClaim(challID, ist)
		if err != nil {
			// In pool
			continue
		}
		if src == sourceID {
			return ist, nil
		}
	}
	return "", fmt.Errorf("instance of challenge %s not found for source %s", challID, sourceID)
}

func InstanceDirectory(challID, identity string) string {
	return filepath.Join(ChallengeDirectory(challID), instanceSubdir, identity)
}

// CheckInstance returns an error if there is no instance with the given ids.
func CheckInstance(challID, identity string) error {
	fpath := filepath.Join(InstanceDirectory(challID, identity), infoFile)
	if _, err := os.Stat(fpath); err != nil {
		return &errs.ErrInstanceExist{
			ChallengeID: challID,
			SourceID:    identity, // XXX mismatch
			Exist:       false,
		}
	}
	return nil
}

func ListInstances(challID string) ([]string, error) {
	challDir := ChallengeDirectory(challID)
	dir, err := os.ReadDir(filepath.Join(challDir, instanceSubdir))
	if err != nil {
		return nil, err
	}
	iids := make([]string, 0, len(dir))
	for _, dfs := range dir {
		iids = append(iids, dfs.Name())
	}
	return iids, nil
}

func LoadInstance(challID, identity string) (*Instance, error) {
	if err := CheckInstance(challID, identity); err != nil {
		return nil, err
	}

	fpath := filepath.Join(InstanceDirectory(challID, identity), infoFile)
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
	idir := InstanceDirectory(ist.ChallengeID, ist.Identity)
	// MkdirAll rather than Mkdir for pooled instances (challenge has not created the directory yet)
	_ = os.MkdirAll(idir, os.ModePerm)

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
	idir := InstanceDirectory(ist.ChallengeID, ist.Identity)
	if err := os.RemoveAll(idir); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}
