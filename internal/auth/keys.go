// SPDX-License-Identifier: AGPL-3.0-only

// Package auth holds API key and user token primitives and the HTTP middleware
// that authenticates them. It knows nothing about storage: callers supply a Lookup.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// Token prefixes.
const (
	PrefixSecretKey = "ohs_sk_"
	PrefixUserToken = "ohs_ut_"
)

const (
	base62    = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	secretLen = 43 // 62^43 > 2^256, so 32 random bytes always fit
)

// NewSecret returns a new secret (prefix + 32 random bytes in base62, zero padded
// to 43 characters) and its SHA-256 hex hash. Only the hash is ever stored.
func NewSecret(prefix string) (full, hash string, err error) {
	if prefix != PrefixSecretKey && prefix != PrefixUserToken {
		return "", "", fmt.Errorf("auth: unknown secret prefix %q", prefix)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("auth: random: %w", err)
	}
	n := new(big.Int).SetBytes(raw)
	base := big.NewInt(62)
	mod := new(big.Int)
	out := make([]byte, secretLen)
	for i := secretLen - 1; i >= 0; i-- {
		n.DivMod(n, base, mod)
		out[i] = base62[mod.Int64()]
	}
	full = prefix + string(out)
	return full, Hash(full), nil
}

// Hash returns the hex SHA-256 of a full secret. Secrets are high entropy, so a
// fast hash is appropriate.
func Hash(full string) string {
	sum := sha256.Sum256([]byte(full))
	return hex.EncodeToString(sum[:])
}
