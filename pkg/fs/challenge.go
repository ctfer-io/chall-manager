package fs

import (
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"
	"go.uber.org/multierr"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// Challenge is the internal model of an API Challenge as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/hash(<id>)/info.json`).
type Challenge struct {
	ID         string            `json:"id"`
	Scenario   string            `json:"scenario"`
	Until      *time.Time        `json:"until,omitempty"`
	Timeout    *time.Duration    `json:"timeout,omitempty"`
	Additional map[string]string `json:"additional,omitempty"`
	Min        int64             `json:"min"`
	Max        int64             `json:"max"`
}

func challengeDirectory(id string) string {
	return filepath.Join(global.Conf.Directory, challSubdir, Hash(id))
}

// CheckChallenge returns an [*errs.ChallengeExist] if there is no challenge with the given id.
// It avoids reading the whole file and loading the corresponding challenge in memory, when not necessary.
func CheckChallenge(id string) error {
	_, err := os.Stat(challengeDirectory(id))
	if err == nil {
		return nil // exist
	}
	if os.IsNotExist(err) {
		return &errs.ChallengeExist{
			ID:    id,
			Exist: false, // does not exist
		}
	}
	return err // internal server error
}

// ListChallenges loads all Challenges.
// It opens every Challenge information file for ID lookup, so usage should be avoided when an alternative exist.
func ListChallenges() (ids []string, merr error) {
	dir, err := os.ReadDir(filepath.Join(global.Conf.Directory, challSubdir))
	if err != nil {
		return
	}
	for _, dfs := range dir {
		id, err := idOfChallenge(dfs.Name())
		if err != nil {
			merr = multierr.Append(merr, err)
			continue
		}
		ids = append(ids, id)
	}
	if merr != nil {
		return nil, merr
	}
	return
}

// LoadChallenge opens the Challenge information file and returns it.
// Returns an [*errs.ChallengeExist] when the Challenge does not exist.
//
// For existence check, please use [CheckChallenge] intead.
func LoadChallenge(id string) (*Challenge, error) {
	fpath := filepath.Join(challengeDirectory(id), infoFile)
	f, err := os.Open(fpath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &errs.ChallengeExist{
				ID:    id,
				Exist: false,
			}
		}
		return nil, err // internal server error
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fschall := &Challenge{}
	if err := dec.Decode(fschall); err != nil {
		return nil, err // internal server error
	}
	return fschall, nil
}

// Save the Challenge, so write it to disk.
func (chall *Challenge) Save() error {
	challDir := challengeDirectory(chall.ID)
	if err := os.MkdirAll(filepath.Join(challDir, instanceSubdir), 0755); err != nil {
		if !os.IsExist(err) {
			return err // internal server error
		}
		// else it is fine, it is a guard rail to ensure the directory exists
	}

	f, err := os.Create(filepath.Join(challDir, infoFile))
	if err != nil {
		return err
	}
	defer fclose(f)

	return json.NewEncoder(f).Encode(chall)
}

// Delete the Challenge, so delete it from disk.
func (chall *Challenge) Delete() error {
	return os.RemoveAll(challengeDirectory(chall.ID))
}

// Lookup for the corresponding ID of a challenge from its hashed ID.
func idOfChallenge(idh string) (string, error) {
	f, err := os.Open(filepath.Join(global.Conf.Directory, challSubdir, idh, infoFile))
	if err != nil {
		return "", err
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fschall := &Challenge{}
	if err := dec.Decode(fschall); err != nil {
		return "", err
	}
	return fschall.ID, nil
}
