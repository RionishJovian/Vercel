package invitations

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// NewToken returns a high-entropy URL-safe invitation token and the only value
// that should be stored in the database: its SHA-256 digest.
func NewToken() (raw string, digest string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate invitation token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	digest = HashToken(raw)
	return raw, digest, nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
