package scenario

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const (
	scenarioDir = "scenario"
)

// Decode (base 64) and unzip the scenario content into the scenario directory
// along the others.
// If files already exist for this challenge, erase it first.
// Returns the directory to look for the Pulumi stack or an error if anything
// went wrong.
func Decode(challDir, scenario string) (string, error) {
	// Create challenge directory, delete previous if any
	cd := filepath.Join(challDir, scenarioDir)
	outDir := ""
	if _, err := os.Stat(cd); err == nil {
		if err := os.RemoveAll(cd); err != nil {
			return "", err
		}
	}
	if err := os.Mkdir(cd, os.ModePerm); err != nil {
		return "", err
	}

	// Decode base64
	b, err := base64.StdEncoding.DecodeString(scenario)
	if err != nil {
		return "", err
	}

	// Unzip content into it
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}
	for _, f := range r.File {
		filePath := filepath.Join(cd, f.Name)

		if f.FileInfo().IsDir() {
			if outDir != "" {
				return "", errors.New("archive contain multiple directories, should not occur")
			}
			outDir = f.Name
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return "", err
			}
			continue
		}

		outFile, err := os.Create(filePath)
		if err != nil {
			return "", err
		}
		defer outFile.Close()

		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()

		if _, err := io.Copy(outFile, rc); err != nil {
			return "", err
		}
	}
	return filepath.Join("scenario", outDir), nil
}
