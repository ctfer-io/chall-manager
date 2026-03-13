package oci

import (
	"context"
)

// Equals compare the references and returns whether they are equal or not.
// To do so, it resolves the digests of the references if exist
func (mg *Manager) Equals(ctx context.Context, ref1, ref2 string) (bool, error) {
	r1, err := mg.parseRef(ctx, ref1)
	if err != nil {
		return false, err
	}
	r2, err := mg.parseRef(ctx, ref2)
	if err != nil {
		return false, err
	}

	return r1 == r2, nil
}

// Parses an OCI reference and finds its digest if necessary.
// Uses the Manager's digest cache if hit, else (miss) will populate it for upcoming calls.
func (mg *Manager) parseRef(ctx context.Context, ref string) (string, error) {
	_, dig, err := mg.resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	return dig, nil
}
