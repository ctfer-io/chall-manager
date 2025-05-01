package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const (
	size = 16
)

// New identity generated and returned.
// The function is not deterministic, but produce a crypto-safe random value.
//
// It has a limited length of 16 thus could be used as a DNS label, while
// remaining most probably unguessable and large enough to scale
// (16 chars ^ 16 runes of hex alphabet = 18 446 744 073 709 551 616 combinations).
func New() string {
	b := make([]byte, size)
	_, _ = rand.Read(b)

	h := sha256.New()
	_, _ = h.Write(b)
	return hex.EncodeToString(h.Sum(nil))[:size]
}
