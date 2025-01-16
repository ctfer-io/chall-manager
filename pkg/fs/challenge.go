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
// filesystem (at `<global.Conf.Directory>/chall/<id>/info.json`).
type Challenge struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
	// must be kept up coherent with directory content as its sha256 sum of base64(zip(content))
	Hash    string            `json:"hash"`
	Until   *time.Time        `json:"until,omitempty"`
	Timeout *time.Duration    `json:"timeout,omitempty"`
	Config  map[string]string `json:"config,omitempty"`
}

func ChallengeDirectory(id string) string {
	return filepath.Join(global.Conf.Directory, challSubdir, Hash(id))
}

// CheckChallenge returns an error if there is no challenge with the given id.
func CheckChallenge(id string) error {
	// Check both directory and the json file -> the scenario can be decoded in parallel
	// of an incoming query, but as it won't be complete, the json file won't be ready.
	dir := ChallengeDirectory(id)
	if _, err := os.Stat(dir); err != nil {
		return &errs.ErrChallengeExist{
			ID:    id,
			Exist: false,
		}
	}
	fpath := filepath.Join(dir, infoFile)
	if _, err := os.Stat(fpath); err != nil {
		return &errs.ErrChallengeExist{
			ID:    id,
			Exist: false,
		}
	}
	return nil
}

func ListChallenges() (ids []string, merr error) {
	dir, err := os.ReadDir(filepath.Join(global.Conf.Directory, challSubdir))
	if err != nil {
		return
	}
	for _, dfs := range dir {
		id, err := idOfChallenge(dfs.Name())
		if err != nil {
			// If challenge does not fully exist yet (scenario is currently decoded
			// and validated but info are not registered), skip it.
			if _, ok := err.(*os.PathError); ok {
				continue
			}
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

func LoadChallenge(id string) (*Challenge, error) {
	if err := CheckChallenge(id); err != nil {
		return nil, err
	}

	fpath := filepath.Join(ChallengeDirectory(id), infoFile)
	f, err := os.Open(fpath)
	if err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fschall := &Challenge{}
	if err := dec.Decode(fschall); err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	return fschall, nil
}

func (chall *Challenge) Save() error {
	challDir := ChallengeDirectory(chall.ID)
	_ = os.Mkdir(challDir, os.ModePerm)
	_ = os.Mkdir(filepath.Join(challDir, instanceSubdir), os.ModePerm)

	fpath := filepath.Join(challDir, infoFile)
	f, err := os.Create(fpath)
	if err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	defer fclose(f)

	enc := json.NewEncoder(f)
	if err := enc.Encode(chall); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}

func (chall *Challenge) Delete() error {
	dir := ChallengeDirectory(chall.ID)
	if err := os.RemoveAll(dir); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}

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
