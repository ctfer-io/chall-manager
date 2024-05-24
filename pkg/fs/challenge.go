package fs

import (
	"os"
	"path/filepath"
	"time"

	json "github.com/goccy/go-json"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// Challenge is the internal model of an API Challenge as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/<id>/info.json`).
type Challenge struct {
	ID        string         `json:"id"`
	Directory string         `json:"directory"`
	Hash      string         `json:"hash"` // must be kept up coherent with directory content as its sha256 sum of base64(zip(content))
	Until     *time.Time     `json:"until,omitempty"`
	Timeout   *time.Duration `json:"timeout,omitempty"`
}

func ChallengeDirectory(id string) string {
	return filepath.Join(global.Conf.Directory, "chall", Hash(id))
}

// CheckChallenge returns an error if there is no challenge with the given id.
func CheckChallenge(id string) error {
	if _, err := os.Stat(ChallengeDirectory(id)); err != nil {
		return &errs.ErrChallengeExist{
			ID:    id,
			Exist: false,
		}
	}
	return nil
}

// IdOfChallenge returns the corresponding ID of the challenge given its hash.
func IdOfChallenge(idh string) (string, error) {
	f, err := os.Open(filepath.Join(global.Conf.Directory, "chall", idh, "info.json"))
	if err != nil {
		return "", &errs.ErrInternal{Sub: err}
	}
	defer fclose(f)

	dec := json.NewDecoder(f)
	fschall := &Challenge{}
	if err := dec.Decode(fschall); err != nil {
		return "", &errs.ErrInternal{Sub: err}
	}
	return fschall.ID, nil
}

func LoadChallenge(id string) (*Challenge, error) {
	if err := CheckChallenge(id); err != nil {
		return nil, err
	}

	fpath := filepath.Join(ChallengeDirectory(id), "info.json")
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

	fpath := filepath.Join(challDir, "info.json")
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
	if err := os.RemoveAll(ChallengeDirectory(chall.ID)); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}
