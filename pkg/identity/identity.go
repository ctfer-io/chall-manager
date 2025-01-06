package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// Compute an indentity with is a unique name depending on the challenge and source
// ids contained within a request to avoid colliding with another instance, and a random
// uuid serving as salt.
//
// It has a limited length of 16 thus could be used as a DNS label, while
// remaining most probably unguessable and large enough to scale
// (16 chars ^ 16 runes of hex alphabet = 18 446 744 073 709 551 616 combinations).
//
// This identity is not predictable as it will internally combine a (crypto)
// random instance id that will get appended in the hash input function.
func Compute(challID, sourceID string) string {
	instanceId := uuid.New().String()

	sha := sha256.New()
	_, _ = sha.Write([]byte(fmt.Sprintf("%s-%s-%s", challID, sourceID, instanceId)))
	b := hex.EncodeToString(sha.Sum(nil))
	return string(b[:16])
}
