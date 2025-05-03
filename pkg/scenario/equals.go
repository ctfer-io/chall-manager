package scenario

import (
	"fmt"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/crane"
)

func Equals(ref1, ref2 string) (bool, error) {
	r1, err := parseRef(ref1)
	if err != nil {
		return false, err
	}
	r2, err := parseRef(ref2)
	if err != nil {
		return false, err
	}

	return r1 == r2, nil
}

func parseRef(ref string) (string, error) {
	// Parse
	rr, err := reference.Parse(ref)
	if err != nil {
		return "", err
	}
	r := rr.(reference.Named)

	// Look for digest
	var dig string
	if cref, ok := r.(reference.Canonical); ok {
		// Digest is already in the ref
		dig = cref.Digest().Encoded()
	} else {
		// Get it from upstream
		opts := []crane.Option{}
		if global.Conf.OCI.Insecure {
			opts = append(opts, crane.Insecure)
		}
		dig, err = crane.Digest(ref, opts...)
		if err != nil {
			return "", err
		}
	}

	// Combine
	return fmt.Sprintf("%s@sha256:%s", r.Name(), dig), nil
}
