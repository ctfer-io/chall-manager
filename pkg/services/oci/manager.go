package oci

import (
	"sync"
)

type Manager struct {
	locks    *sync.Map
	digCache *sync.Map

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
		digCache:      &sync.Map{},
		insecure:      insecure,
		username:      username,
		password:      password,
		cacheOverride: cacheOverride,
	}
}
