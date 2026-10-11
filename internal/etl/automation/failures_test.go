// SPDX-License-Identifier: AGPL-3.0-only

package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCountFailures(t *testing.T) {
	run := func(fn func(context.Context) error, ctx context.Context) (failures int64, panicked bool) {
		m := &Metrics{}
		defer func() {
			panicked = recover() != nil
			failures = m.RunFailures.Load()
		}()
		_ = countFailures(m, fn)(ctx)
		return
	}
	waitCtx := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

	// A run that hit its timeout is a failure.
	timed, cancelT := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelT()
	f, p := run(waitCtx, timed)
	require.EqualValues(t, 1, f)
	require.False(t, p)

	// A run cut by shutdown is not.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	f, p = run(waitCtx, cancelled)
	require.EqualValues(t, 0, f)
	require.False(t, p)

	// An ordinary error is.
	f, _ = run(func(context.Context) error { return errors.New("boom") }, context.Background())
	require.EqualValues(t, 1, f)

	// A panic is, and still propagates to the scheduler.
	f, p = run(func(context.Context) error { panic("kaboom") }, context.Background())
	require.EqualValues(t, 1, f)
	require.True(t, p)

	// Success is not.
	f, _ = run(func(context.Context) error { return nil }, context.Background())
	require.EqualValues(t, 0, f)
}
