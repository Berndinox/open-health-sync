// SPDX-License-Identifier: AGPL-3.0-only

package rawstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestKey(t *testing.T) {
	tn := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	us := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	require.Equal(t,
		"raw/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/apple_health/2026-10-10/b1_3.ndjson.gz",
		Key(tn, us, "apple_health", day, "b1", 3))
	// The day comes from the time's own calendar date, not UTC.
	pst := time.FixedZone("PST", -8*3600)
	late := time.Date(2026, 10, 10, 23, 30, 0, 0, pst)
	require.Contains(t, Key(tn, us, "p", late, "b", 0), "/2026-10-10/")
}

func read(t *testing.T, s Store, key string) string {
	t.Helper()
	rc, err := s.Get(context.Background(), key)
	require.NoError(t, err)
	defer rc.Close()
	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	return string(b)
}

// RunConformance exercises the Store contract. Every driver must pass it.
func RunConformance(t *testing.T, s Store) {
	ctx := context.Background()

	t.Run("round trip", func(t *testing.T) {
		require.NoError(t, s.Put(ctx, "raw/a/one.ndjson.gz", strings.NewReader("hello")))
		require.Equal(t, "hello", read(t, s, "raw/a/one.ndjson.gz"))
	})

	t.Run("second put is ErrExists and keeps the original", func(t *testing.T) {
		require.NoError(t, s.Put(ctx, "raw/b/x", strings.NewReader("first")))
		err := s.Put(ctx, "raw/b/x", strings.NewReader("second"))
		require.True(t, errors.Is(err, ErrExists), "got %v", err)
		require.Equal(t, "first", read(t, s, "raw/b/x"))
	})

	t.Run("missing key is ErrNotFound", func(t *testing.T) {
		_, err := s.Get(ctx, "raw/nope/missing")
		require.True(t, errors.Is(err, ErrNotFound), "got %v", err)
	})

	t.Run("list by prefix is sorted", func(t *testing.T) {
		for _, k := range []string{"raw/l/c", "raw/l/a", "raw/l/b/z", "raw/lx/q"} {
			require.NoError(t, s.Put(ctx, k, strings.NewReader("v")))
		}
		got, err := s.List(ctx, "raw/l/")
		require.NoError(t, err)
		require.Equal(t, []string{"raw/l/a", "raw/l/b/z", "raw/l/c"}, got)
		none, err := s.List(ctx, "raw/zzz/")
		require.NoError(t, err)
		require.Empty(t, none)
	})

	t.Run("walk pages the same keys as list", func(t *testing.T) {
		want := []string{}
		for i := 0; i < WalkPageSize+50; i++ {
			k := fmt.Sprintf("raw/w/%05d", i)
			want = append(want, k)
			require.NoError(t, s.Put(ctx, k, strings.NewReader("v")))
		}
		require.NoError(t, s.Put(ctx, "raw/wx/other", strings.NewReader("v")))
		var got []string
		pages := 0
		require.NoError(t, s.Walk(ctx, "raw/w/", func(keys []string) error {
			pages++
			require.LessOrEqual(t, len(keys), WalkPageSize)
			require.True(t, sort.StringsAreSorted(keys))
			got = append(got, keys...)
			return nil
		}))
		require.GreaterOrEqual(t, pages, 2)
		sort.Strings(got)
		require.Equal(t, want, got)

		// An error from fn stops the walk and comes back.
		boom := errors.New("boom")
		calls := 0
		require.ErrorIs(t, s.Walk(ctx, "raw/w/", func([]string) error { calls++; return boom }), boom)
		require.Equal(t, 1, calls)

		// Nothing under the prefix: fn is never called.
		require.NoError(t, s.Walk(ctx, "raw/walk-none/", func([]string) error { t.Fatal("called"); return nil }))
		require.ErrorIs(t, s.Walk(ctx, "raw/../", func([]string) error { return nil }), ErrInvalidKey)
	})

	t.Run("delete prefix removes only that prefix", func(t *testing.T) {
		for _, k := range []string{"raw/d/1/a", "raw/d/1/b", "raw/d/2/a", "raw/d/10/a"} {
			require.NoError(t, s.Put(ctx, k, strings.NewReader("v")))
		}
		require.NoError(t, s.DeletePrefix(ctx, "raw/d/1/"))
		got, err := s.List(ctx, "raw/d/")
		require.NoError(t, err)
		require.Equal(t, []string{"raw/d/10/a", "raw/d/2/a"}, got)
		// A deleted key can be written again.
		require.NoError(t, s.Put(ctx, "raw/d/1/a", strings.NewReader("again")))
		require.NoError(t, s.DeletePrefix(ctx, "raw/never/a/"))
		for _, p := range []string{"", "raw/", "raw/tenant", "raw/tenant/", "raw/t/u"} {
			require.ErrorIs(t, s.DeletePrefix(ctx, p), ErrInvalidKey, p)
		}
	})

	t.Run("bad keys are rejected", func(t *testing.T) {
		for _, k := range []string{"raw/../etc/passwd", "../x", "/abs/key", "raw/a..b", "", "raw//x", "raw/./x", "raw/x/", "raw/.ohs-tmp-1"} {
			require.ErrorIs(t, s.Put(ctx, k, strings.NewReader("v")), ErrInvalidKey, k)
			_, err := s.Get(ctx, k)
			require.ErrorIs(t, err, ErrInvalidKey, k)
		}
		for _, p := range []string{"raw/../", "/raw", "raw//x", "raw/./"} {
			_, err := s.List(ctx, p)
			require.ErrorIs(t, err, ErrInvalidKey, p)
			require.ErrorIs(t, s.DeletePrefix(ctx, p), ErrInvalidKey, p)
		}
	})

	t.Run("concurrent puts of one key: exactly one wins", func(t *testing.T) {
		const n = 8
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			go func() { errs <- s.Put(ctx, "raw/race/k", strings.NewReader(fmt.Sprintf("writer-%d", i))) }()
		}
		wins := 0
		for i := 0; i < n; i++ {
			err := <-errs
			if err == nil {
				wins++
			} else {
				require.ErrorIs(t, err, ErrExists)
			}
		}
		require.Equal(t, 1, wins)
		got := read(t, s, "raw/race/k")
		require.Regexp(t, `^writer-[0-7]$`, got)
	})
}
