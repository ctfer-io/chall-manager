package fs

import (
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// Instance is the internal model of an API Instance as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/hash(<id>)/instance/hash(<id>)/info.json`)
type Instance struct {
	Identity       string            `json:"identity"`
	ChallengeID    string            `json:"challenge_id"`
	State          any               `json:"state"`
	Since          time.Time         `json:"since"`
	LastRenew      time.Time         `json:"last_renew"`
	Until          *time.Time        `json:"until,omitempty"`
	ConnectionInfo string            `json:"connection_info"`
	Flags          []string          `json:"flags,omitempty"`
	Additional     map[string]string `json:"additional,omitempty"`
}

// Claim a challenge instance (by its identity) for a source.
func Claim(challID, identity, sourceID string) error {
	fsist := &Instance{
		ChallengeID: challID,
		Identity:    identity,
	}
	return fsist.Claim(sourceID)
}

// Claim the instance for a source.
func (ist *Instance) Claim(sourceID string) error {
	claimPath := filepath.Join(instanceDirectory(ist.ChallengeID, ist.Identity), claimFile)
	return os.WriteFile(claimPath, []byte(sourceID), 0600)
}

// LookupClaim returns the source that claims an instance.
func LookupClaim(challID, identity string) (string, error) {
	b, err := os.ReadFile(filepath.Join(instanceDirectory(challID, identity), claimFile))
	if err == nil {
		return string(b), nil // exist
	}
	if os.IsNotExist(err) {
		return "", &errs.InstanceExist{
			ChallengeID: challID,
			SourceID:    identity, // XXX should not use the source ID
			Exist:       false,
		}
	}
	return "", err
}

// FindInstance loads all Instances until finding the one claimed by a source.
// It opens every Instance information file for claim lookup, so usage should be avoided when an alternative exist.
//
// Returns the identity claimed by the sourceID for the challenge, or an error.
// Errors could be of type [*errors.InstanceExist] if it was not found, or anything else if something
// unexpected happened (e.g., filesystem read failure).
func FindInstance(challID, sourceID string) (string, error) {
	ists, err := ListInstances(challID) // XXX don't load all before looking for it, do it in one pass
	if err != nil {
		return "", err
	}
	for _, ist := range ists {
		src, err := LookupClaim(challID, ist)
		if err != nil {
			if _, ok := err.(*errs.InstanceExist); ok {
				// In pool
				continue
			}
			return "", err
		}
		if src == sourceID {
			return ist, nil
		}
	}
	return "", &errs.InstanceExist{
		ChallengeID: challID,
		SourceID:    sourceID,
		Exist:       false,
	}
}

func instanceDirectory(challID, identity string) string {
	return filepath.Join(challengeDirectory(challID), instanceSubdir, identity)
}

// CheckInstance returns an error if there is no instance with the given ids.
func CheckInstance(challID, identity string) error {
	_, err := os.Stat(filepath.Join(instanceDirectory(challID, identity), infoFile))
	if err == nil {
		return nil // exist
	}
	if os.IsNotExist(err) {
		return &errs.InstanceExist{
			ChallengeID: challID,
			SourceID:    identity, // XXX should not use the source ID
			Exist:       false,
		}
	}
	return err // internal server error
}

func ListInstances(challID string) ([]string, error) {
	challDir := challengeDirectory(challID)
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

	fpath := filepath.Join(instanceDirectory(challID, identity), infoFile)
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fsist := &Instance{}
	if err := dec.Decode(fsist); err != nil {
		return nil, err
	}
	return fsist, nil
}

func (ist *Instance) Save() error {
	idir := instanceDirectory(ist.ChallengeID, ist.Identity)
	// MkdirAll rather than Mkdir for pooled instances (challenge has not created the directory yet)
	_ = os.MkdirAll(idir, os.ModePerm)

	fpath := filepath.Join(idir, infoFile)
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}
	defer fclose(f)

	enc := json.NewEncoder(f)
	if err := enc.Encode(ist); err != nil {
		return err
	}
	return nil
}

func (ist *Instance) Delete() error {
	if err := os.RemoveAll(instanceDirectory(ist.ChallengeID, ist.Identity)); err != nil {
		return err
	}
	return nil
}
