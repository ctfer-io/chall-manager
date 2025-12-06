package oci

import (
	"os"
	"path/filepath"
)

var (
	cacheDir = filepath.Join(os.Getenv("HOME"), ".cache", "chall-manager")
)

// CacheDir returns the cache directory in which to load scenarios.
func (mg *Manager) cacheDir() string {
	if mg.cacheOverride != "" {
		return mg.cacheOverride
	}

	// guarantee that even if the cache override "/root", "/home/someone", or nothing, it catches
	// that it should be an absolute path to avoid interpretations.
	// This has been manually tested, worked fine, but enables checking it works even if
	// the Docker image changes in the future (e.g. minimization), or the Go behavior
	// changes (which should not, but the check is not costful so let's do it).
	if !filepath.IsAbs(cacheDir) {
		panic("cache directory is not absolute")
	}

	return cacheDir
}
