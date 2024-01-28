package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ctfer-io/chall-manager/global"
)

// identity produce a unique name depending on the launch request such that
// it does not collide with another instance, and with the configured salt.
// It has a limited length of 32 thus could be used as a DNS label.
func Compute(challID, sourceID string) string {
	sha := sha256.New()
	_, _ = sha.Write([]byte(fmt.Sprintf("%s-%s-%s", challID, sourceID, global.Conf.Salt)))
	b := hex.EncodeToString(sha.Sum(nil))
	return string(b[:32])
}
