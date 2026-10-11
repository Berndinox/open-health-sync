// SPDX-License-Identifier: AGPL-3.0-only

// Package crypto seals provider OAuth tokens at rest with AES-256-GCM.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// Box seals and opens small secrets. The sealed form is nonce || ciphertext || tag.
type Box struct {
	aead cipher.AEAD
}

// NewBox builds a Box from a 32 byte key.
func NewBox(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plain with a fresh random nonce and no additional data.
func (b *Box) Seal(plain []byte) ([]byte, error) { return b.SealFor(plain, nil) }

// SealFor is Seal bound to aad (for example tenant, user and provider ids): the
// value only opens with the same aad, so a sealed token cannot be moved to another row.
func (b *Box) SealFor(plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize(), b.aead.NonceSize()+len(plain)+b.aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, plain, aad), nil
}

// Open decrypts and authenticates a value produced by Seal.
func (b *Box) Open(sealed []byte) ([]byte, error) { return b.OpenFor(sealed, nil) }

// OpenFor opens a value produced by SealFor with the same aad.
func (b *Box) OpenFor(sealed, aad []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n+b.aead.Overhead() {
		return nil, errors.New("crypto: sealed value too short")
	}
	plain, err := b.aead.Open(nil, sealed[:n], sealed[n:], aad)
	if err != nil {
		return nil, errors.New("crypto: cannot open sealed value")
	}
	return plain, nil
}
