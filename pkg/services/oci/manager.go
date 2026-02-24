package oci

import (
	"sync"
)

type Manager struct {
	// A concurrency-safe top-level locks map. Ensures concurrent calls synchronization to avoid duplicate
	// OCI digest lookup and pulls.
	locks *sync.Map

	// A local digest cache. No TTL in place, so an app reboot might be necessary to reload it in case of
	// reference override. To avoid this, we recommend you pin the references by hash (i.e., my/ref@<hash>).
	digCache map[string]*cacheEntry

	insecure           bool
	username, password string

	cacheOverride string
}

func NewManager(
	insecure bool,
	username, password string,
	cacheOverride string,
) *Manager {
	return &Manager{
		locks:         &sync.Map{},
		digCache:      map[string]*cacheEntry{},
		insecure:      insecure,
		username:      username,
		password:      password,
		cacheOverride: cacheOverride,
	}
}
