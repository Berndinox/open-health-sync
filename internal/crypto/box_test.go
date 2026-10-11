// SPDX-License-Identifier: AGPL-3.0-only

package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func key32() []byte { return bytes.Repeat([]byte{7}, 32) }

func TestBoxRoundTrip(t *testing.T) {
	b, err := NewBox(key32())
	require.NoError(t, err)
	sealed, err := b.Seal([]byte("refresh-token"))
	require.NoError(t, err)
	require.NotContains(t, string(sealed), "refresh-token")
	plain, err := b.Open(sealed)
	require.NoError(t, err)
	require.Equal(t, []byte("refresh-token"), plain)
}

func TestBoxNonceIsFresh(t *testing.T) {
	b, _ := NewBox(key32())
	a, _ := b.Seal([]byte("x"))
	c, _ := b.Seal([]byte("x"))
	require.NotEqual(t, a, c)
}

func TestBoxTamperDetected(t *testing.T) {
	b, _ := NewBox(key32())
	sealed, _ := b.Seal([]byte("secret"))
	for i := range sealed {
		bad := bytes.Clone(sealed)
		bad[i] ^= 1
		_, err := b.Open(bad)
		require.Error(t, err, "flip at byte %d", i)
	}
}

func TestBoxWrongKey(t *testing.T) {
	b, _ := NewBox(key32())
	sealed, _ := b.Seal([]byte("secret"))
	other, _ := NewBox(bytes.Repeat([]byte{9}, 32))
	_, err := other.Open(sealed)
	require.Error(t, err)
}

func TestBoxShortInput(t *testing.T) {
	b, _ := NewBox(key32())
	_, err := b.Open([]byte{1, 2, 3})
	require.Error(t, err)
	_, err = b.Open(nil)
	require.Error(t, err)
}

func TestNewBoxKeyLength(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		_, err := NewBox(make([]byte, n))
		require.Error(t, err, "len %d", n)
	}
}

func TestBoxAAD(t *testing.T) {
	b, _ := NewBox(key32())
	sealed, err := b.SealFor([]byte("tok"), []byte("tenant-a"))
	require.NoError(t, err)
	got, err := b.OpenFor(sealed, []byte("tenant-a"))
	require.NoError(t, err)
	require.Equal(t, []byte("tok"), got)
	_, err = b.OpenFor(sealed, []byte("tenant-b"))
	require.Error(t, err)
	_, err = b.Open(sealed)
	require.Error(t, err)
	plain, _ := b.Seal([]byte("x"))
	_, err = b.OpenFor(plain, []byte("a"))
	require.Error(t, err)
}
