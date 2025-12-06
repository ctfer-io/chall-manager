package oci

import (
	"sync"
)

type Manager struct {
	locks    *sync.Map
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
