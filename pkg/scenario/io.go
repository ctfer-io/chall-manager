package scenario

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/pkg/errors"
)

const (
	scenarioDir = "scenario"
	scenSize    = 1 << 30 // 1Gb
)

// Decode (base 64) and unzip the scenario content into the scenario directory
// along the others.
// If files already exist for this challenge, erase it first.
// Returns the directory to look for the Pulumi stack or an error if anything
// went wrong.
// Error is of type *errors.ErrInternal if related to file-system errors, else
// a meaningfull error to return to the API call.
func Decode(ctx context.Context, challDir, scenario string) (string, error) {
	// Create challenge directory, delete previous if any
	randDir := randName()

	cd := filepath.Join(challDir, scenarioDir, randDir)
	if _, err := os.Stat(cd); err == nil {
		return cd, &errs.ErrInternal{Sub: fmt.Errorf("scenario directory %s already exist", cd)}
	}
	if err := os.MkdirAll(cd, os.ModePerm); err != nil {
		return cd, &errs.ErrInternal{Sub: err}
	}

	// Decode base64
	b, err := base64.StdEncoding.DecodeString(scenario)
	if err != nil {
		return cd, errors.Wrap(err, "base64 decoding")
	}

	// Unzip content into it
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return cd, errors.Wrap(err, "base64 decoded invalid zip archive")
	}

	// Safely decompress the archive
	dec := NewDecompressor(&Options{
		MaxSize: scenSize,
	})
	outDir, err := dec.Unzip(r, cd)
	if err != nil {
		return cd, err
	}

	// Validate its content
	return outDir, Validate(ctx, outDir)
}
