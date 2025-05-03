package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/distribution/reference"
	json "github.com/goccy/go-json"
	"github.com/google/go-containerregistry/pkg/crane"
	"go.uber.org/multierr"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
)

// Challenge is the internal model of an API Challenge as it is stored on the
// filesystem (at `<global.Conf.Directory>/chall/<id>/info.json`).
type Challenge struct {
	ID         string            `json:"id"`
	Scenario   string            `json:"scenario"`
	Directory  string            `json:"directory"`
	Until      *time.Time        `json:"until,omitempty"`
	Timeout    *time.Duration    `json:"timeout,omitempty"`
	Additional map[string]string `json:"additional,omitempty"`
	Min        int64             `json:"min"`
	Max        int64             `json:"max"`
}

// RefDirectory returns the directory of a given reference.
// This reference can not contain the digest, but will be fetched.
// Format is `<global.Conf.Directory>/chall/<hash(image@sha256:digest)>`.
func RefDirectory(id, ref string) (string, error) {
	rr, err := reference.Parse(ref)
	if err != nil {
		return "", err
	}
	r, ok := rr.(reference.Named)
	if !ok {
		return "", errors.New("invalid reference format, may miss a tag")
	}

	// Look for digest
	var dig string
	if cref, ok := r.(reference.Canonical); ok {
		// Digest is already in the ref
		dig = cref.Digest().Encoded()
	} else {
		// Get it from upstream
		dig, err = crane.Digest(ref)
		if err != nil {
			return "", err
		}
	}

	// Combine as a directory.
	return filepath.Join(
		ChallengeDirectory(id),
		Hash(fmt.Sprintf("%s@%s", r.Name(), dig)),
	), nil
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

// Wash the Pulumi.<identity>.yaml file of a challenge given its directory,
// as it is not performed by Pulumi on stack destroy but is an implied
// operation in our context.
func Wash(challDir, identity string) error {
	if err := os.Remove(filepath.Join(challDir, fmt.Sprintf("Pulumi.%s.yaml", identity))); err != nil {
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
