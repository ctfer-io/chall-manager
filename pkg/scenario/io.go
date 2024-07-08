package scenario

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/pkg/errors"
)

const (
	scenarioDir = "scenario"
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
	outDir := ""
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
	for _, f := range r.File {
		filePath := filepath.Join(cd, f.Name)
		if f.FileInfo().IsDir() {
			continue
		}

		// Save output directory i.e. the directory containing the Pulumi.yaml file,
		// the scenario entrypoint.
		if filepath.Base(filePath) == "Pulumi.yaml" {
			if outDir != "" {
				return cd, errors.New("archive contain multiple Pulumi.yaml file, can't easily determine entrypoint")
			}
			outDir = filepath.Dir(filePath)
		}

		// If the file is in a sub-directory, create it
		dir := filepath.Dir(filePath)
		if _, err := os.Stat(dir); err != nil {
			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				return cd, &errs.ErrInternal{Sub: err}
			}
		}

		// Create and write the file
		outFile, err := os.Create(filePath)
		if err != nil {
			return cd, &errs.ErrInternal{Sub: err}
		}
		defer outFile.Close()

		rc, err := f.Open()
		if err != nil {
			return cd, &errs.ErrInternal{Sub: err}
		}
		defer rc.Close()

		if _, err := io.Copy(outFile, rc); err != nil {
			return cd, &errs.ErrInternal{Sub: err}
		}
	}

	return outDir, Validate(ctx, outDir)
}
