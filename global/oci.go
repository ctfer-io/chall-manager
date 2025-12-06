package global

import (
	"sync"

	"github.com/ctfer-io/chall-manager/pkg/services/oci"
)

var (
	ociManager *oci.Manager
	ociOnce    sync.Once
)

func GetOCIManager() *oci.Manager {
	ociOnce.Do(func() {
		ociManager = oci.NewManager(
			Conf.OCI.Insecure,
			Conf.OCI.Username, Conf.OCI.Password,
			Conf.Cache,
		)
	})
	return ociManager
}
