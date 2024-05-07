package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

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

func LoadChallenge(id string) (*Challenge, error) {
	challDir := filepath.Join(global.Conf.Directory, "chall", id)
	fpath := filepath.Join(challDir, "info.json")
	if _, err := os.Stat(fpath); err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	cb, err := os.ReadFile(fpath)
	if err != nil {
		return nil, errs.ErrChallengeExist{
			ID:    id,
			Exist: false,
		}
	}
	fschall := &Challenge{}
	if err := json.Unmarshal(cb, fschall); err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}
	return fschall, nil
}

func (chall *Challenge) Save() error {
	challDir := filepath.Join(global.Conf.Directory, "chall", chall.ID)
	fsb, err := json.Marshal(chall)
	if err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	if err := os.WriteFile(filepath.Join(challDir, "info.json"), fsb, 0644); err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	return nil
}
