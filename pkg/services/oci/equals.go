package oci

import (
	"fmt"

	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
)

func (mg *Manager) Equals(ref1, ref2 string) (bool, error) {
	r1, err := mg.parseRef(ref1)
	if err != nil {
		return false, err
	}
	r2, err := mg.parseRef(ref2)
	if err != nil {
		return false, err
	}

	return r1 == r2, nil
}

func (mg *Manager) parseRef(ref string) (string, error) {
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
		if mg.insecure {
			opts = append(opts, crane.Insecure)
		}
		if mg.username != "" && mg.password != "" {
			opts = append(opts, crane.WithAuth(&authn.Basic{
				Username: mg.username,
				Password: mg.password,
			}))
		}
		dig, err = crane.Digest(ref, opts...)
		if err != nil {
			return "", err
		}
	}

	// Combine
	return fmt.Sprintf("%s@sha256:%s", r.Name(), dig), nil
}
