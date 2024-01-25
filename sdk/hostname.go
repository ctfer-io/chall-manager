package sdk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Hostname deterministically provides a hostname based on the scenario request info.
// Its intended use is with an ingress such that other players won't be able to guess
// the other instances routes (avoid cheating through instance DoS).
// The `sourceID` is the user or team ID, depending on the `mode` the CTF runs in.
func Hostname(challID, sourceID, hostname string) string {
	sha := sha256.New()
	_, _ = sha.Write([]byte(fmt.Sprintf("%s-%s", challID, sourceID)))
	b := hex.EncodeToString(sha.Sum(nil))
	return fmt.Sprintf("%s.%s", b, hostname)
}
