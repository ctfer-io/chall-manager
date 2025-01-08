package scenario

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/pkg/errors"
)

const (
	blockSize = 1 << 13 // arbitrary
)

// Decompressor handle the load of
type Decompressor struct {
	*Options
}

type Options struct {
	MaxSize int64

	currSize int64
}

// NewDecompressor constructs a fresh Decompressor.
func NewDecompressor(opts *Options) *Decompressor {
	if opts == nil {
		opts = &Options{}
	}
	return &Decompressor{
		Options: opts,
	}
}

// Unzip extracts the content of the zip reader into cd.
// It returns the directory it extracted into for Pulumi to use,
// or an error if anything unexpected happens.
func (dec *Decompressor) Unzip(r *zip.Reader, cd string) (string, error) {
	outDir := ""
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		filePath, err := sanitizeArchivePath(cd, f.Name)
		if err != nil {
			return cd, &errs.ErrInternal{Sub: err}
		}

		// Save output directory i.e. the directory containing the Pulumi.yaml file,
		// the scenario entrypoint.
		base := filepath.Base(filePath)
		if base == "Pulumi.yaml" || base == "Pulumi.yml" {
			if outDir != "" {
				return cd, errors.New("archive contain multiple Pulumi yaml/yml file, can't easily determine entrypoint")
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
		if err := dec.copyTo(f, filePath); err != nil {
			return cd, &errs.ErrInternal{Sub: err}
		}
	}

	return outDir, nil
}

func sanitizeArchivePath(d, t string) (v string, err error) {
	v = filepath.Join(d, t)
	if strings.HasPrefix(v, filepath.Clean(d)) {
		return v, nil
	}
	return "", &ErrPathTainted{
		Path: t,
	}
}

func (dec *Decompressor) copyTo(f *zip.File, filePath string) error {
	outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, f.Mode())
	if err != nil {
		return err
	}
	defer outFile.Close()

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	for {
		n, err := io.CopyN(outFile, rc, blockSize)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		dec.currSize += n

		if dec.MaxSize > 0 && dec.currSize > dec.MaxSize {
			return ErrTooLargeContent{
				MaxSize: dec.MaxSize,
			}
		}
	}
}

// ErrPathTainted is returned when a potential zip slip is detected
// through an unzip.
type ErrPathTainted struct {
	Path string
}

func (err ErrPathTainted) Error() string {
	return fmt.Sprintf("filepath is tainted: %s", err.Path)
}

var _ error = (*ErrPathTainted)(nil)

// ErrTooLargeContent is returned when a too large zip is processed
// (e.g. a zip bomb).
type ErrTooLargeContent struct {
	MaxSize int64
}

func (err ErrTooLargeContent) Error() string {
	return fmt.Sprintf("too large archive content, maximum is %d", err.MaxSize)
}

var _ error = (*ErrTooLargeContent)(nil)
