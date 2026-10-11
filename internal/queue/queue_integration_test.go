// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/db"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := db.Open(context.Background(), db.TestDB(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
}

func inTx(t *testing.T, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(ctx))
}

func enqueueETL(t *testing.T, pool *pgxpool.Pool, j ETLJob) {
	t.Helper()
	inTx(t, pool, func(tx pgx.Tx) error { return EnqueueETL(context.Background(), tx, j) })
}

func etlJob(i int, tier string) ETLJob {
	return ETLJob{TenantID: uuid.New(), UserID: uuid.New(), Provider: "apple_health", Key: fmt.Sprintf("raw/k%d", i), Tier: tier}
}

func claimOne(t *testing.T, q *ETL) ETLJob {
	t.Helper()
	got, err := q.Claim(context.Background(), "standard", 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	return got[0]
}

func TestETLEnqueueClaimComplete(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	in := etlJob(1, "standard")
	enqueueETL(t, pool, in)

	got, err := q.Claim(ctx, "standard", 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	g := got[0]
	require.Equal(t, in.TenantID, g.TenantID)
	require.Equal(t, in.UserID, g.UserID)
	require.Equal(t, in.Provider, g.Provider)
	require.Equal(t, in.Key, g.Key)
	require.Equal(t, "standard", g.Tier)
	require.Zero(t, g.Attempts)
	require.NotZero(t, g.ID)
	require.False(t, g.ClaimedAt.IsZero())

	again, err := q.Claim(ctx, "standard", 5)
	require.NoError(t, err)
	require.Empty(t, again, "running job claimed twice")

	require.NoError(t, q.Complete(ctx, g))
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM ohs_sync.etl_jobs WHERE id=$1`, g.ID).Scan(&status))
	require.Equal(t, "done", status)
}

func TestETLEmptyAndTierAndBatch(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	got, err := q.Claim(ctx, "standard", 5)
	require.NoError(t, err)
	require.Empty(t, got)
	for i := 0; i < 5; i++ {
		enqueueETL(t, pool, etlJob(i, "standard"))
	}
	enqueueETL(t, pool, etlJob(99, "high_freq"))
	got, err = q.Claim(ctx, "standard", 3)
	require.NoError(t, err)
	require.Len(t, got, 3)
	hf, err := q.Claim(ctx, "high_freq", 10)
	require.NoError(t, err)
	require.Len(t, hf, 1)
	require.Equal(t, "raw/k99", hf[0].Key)
	require.Equal(t, "high_freq", hf[0].Tier)
}

func TestETLDefaultTierIsStandard(t *testing.T) {
	pool := testPool(t)
	enqueueETL(t, pool, etlJob(1, ""))
	claimOne(t, NewETL(pool))
}

func TestEnqueueInTxRollsBack(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, EnqueueETL(ctx, tx, etlJob(1, "standard")))
	require.NoError(t, tx.Rollback(ctx))
	got, err := NewETL(pool).Claim(ctx, "standard", 5)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestClaimSkipLocked(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	for i := 0; i < 10; i++ {
		enqueueETL(t, pool, etlJob(i, "standard"))
	}

	// A peer holds row locks on 5 of the 10 jobs. Without SKIP LOCKED the
	// claim would block on them; with it the other 5 come back at once.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT id FROM ohs_sync.etl_jobs ORDER BY id LIMIT 5 FOR UPDATE`)
	require.NoError(t, err)

	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	got, err := q.Claim(cctx, "standard", 10)
	require.NoError(t, err)
	require.Len(t, got, 5)
	var locked int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM ohs_sync.etl_jobs WHERE status='pending'`).Scan(&locked))
	require.Equal(t, 5, locked)
	require.NoError(t, tx.Rollback(ctx))

	rest, err := q.Claim(ctx, "standard", 10)
	require.NoError(t, err)
	require.Len(t, rest, 5)
}

func TestConcurrentClaimsDisjoint(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	for i := 0; i < 10; i++ {
		enqueueETL(t, pool, etlJob(i, "standard"))
	}
	var wg sync.WaitGroup
	res := make([][]ETLJob, 2)
	errs := make([]error, 2)
	for w := range res {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[w], errs[w] = q.Claim(ctx, "standard", 10)
		}()
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	seen := map[int64]bool{}
	for _, r := range res {
		for _, j := range r {
			require.False(t, seen[j.ID], "job %d claimed twice", j.ID)
			seen[j.ID] = true
		}
	}
	require.Len(t, seen, 10)
}

func TestVisibilityTimeoutReclaims(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	first := claimOne(t, q)
	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET claimed_at = now() - interval '9 minutes'`)
	got, err := q.Claim(ctx, "standard", 1)
	require.NoError(t, err)
	require.Empty(t, got, "reclaimed too early")
	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET claimed_at = now() - interval '11 minutes'`)
	got, err = q.Claim(ctx, "standard", 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, first.ID, got[0].ID)
	require.Equal(t, 1, got[0].Attempts, "reclaim should burn an attempt")
}

func TestStaleWorkerCannotSettleReclaimedJob(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	a := claimOne(t, q)
	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET claimed_at = now() - interval '11 minutes'`)
	b := claimOne(t, q)
	require.Equal(t, a.ID, b.ID)
	require.NotEqual(t, a.ClaimedAt, b.ClaimedAt)

	require.ErrorIs(t, q.Complete(ctx, a), ErrNotRunning)
	require.ErrorIs(t, q.Fail(ctx, a, errors.New("late")), ErrNotRunning)

	var status string
	var attempts int
	var claimed time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, attempts, claimed_at FROM ohs_sync.etl_jobs`).Scan(&status, &attempts, &claimed))
	require.Equal(t, "running", status)
	require.Equal(t, b.Attempts, attempts)
	require.True(t, claimed.Equal(b.ClaimedAt))

	require.NoError(t, q.Complete(ctx, b))
}

func TestSettleNotRunningReturnsErr(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	j := claimOne(t, q)
	require.NoError(t, q.Complete(ctx, j))
	require.ErrorIs(t, q.Complete(ctx, j), ErrNotRunning, "done")
	require.ErrorIs(t, q.Fail(ctx, j, errors.New("x")), ErrNotRunning, "done")

	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET status='parked'`)
	require.ErrorIs(t, q.Complete(ctx, j), ErrNotRunning, "parked")

	require.ErrorIs(t, q.Complete(ctx, ETLJob{ID: 424242, ClaimedAt: time.Now()}), ErrNotRunning, "unknown")
	require.ErrorIs(t, q.Fail(ctx, ETLJob{ID: 424242, ClaimedAt: time.Now()}, nil), ErrNotRunning, "unknown")
}

func TestFailBackoff(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	j := claimOne(t, q)
	require.NoError(t, q.Fail(ctx, j, errors.New("boom")))
	var status, lastErr string
	var attempts int
	var wait float64
	scan := func() {
		require.NoError(t, pool.QueryRow(ctx, `SELECT status, attempts, last_error, extract(epoch FROM not_before - now()) FROM ohs_sync.etl_jobs`).
			Scan(&status, &attempts, &lastErr, &wait))
	}
	scan()
	require.Equal(t, "pending", status)
	require.Equal(t, 1, attempts)
	require.Equal(t, "boom", lastErr)
	require.InDelta(t, 30, wait, 3)

	got, err := q.Claim(ctx, "standard", 1)
	require.NoError(t, err)
	require.Empty(t, got, "claimable during backoff")
	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET not_before = now()`)
	j = claimOne(t, q)
	require.Equal(t, 1, j.Attempts)
	require.NoError(t, q.Fail(ctx, j, errors.New("boom2")))
	scan()
	require.InDelta(t, 60, wait, 3)
}

func TestFailParksAfterFive(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	for i := 1; i <= 5; i++ {
		mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET not_before = now() WHERE status='pending'`)
		require.NoError(t, q.Fail(ctx, claimOne(t, q), errors.New("nope")))
	}
	var status string
	var attempts int
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, attempts FROM ohs_sync.etl_jobs`).Scan(&status, &attempts))
	require.Equal(t, "parked", status)
	require.Equal(t, 5, attempts)
	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET not_before = now()`)
	got, err := q.Claim(ctx, "standard", 1)
	require.NoError(t, err)
	require.Empty(t, got, "parked job claimed")
}

func TestFailStoresMultibyteNulError(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	j := claimOne(t, q)
	// 3-byte runes straddle the 1000 byte cut; NUL bytes would be rejected by Postgres.
	msg := "x\x00" + strings.Repeat("€", 600) + "\x00end" + string([]byte{0xff})
	require.NoError(t, q.Fail(ctx, j, errors.New(msg)))
	var lastErr string
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_error FROM ohs_sync.etl_jobs`).Scan(&lastErr))
	require.LessOrEqual(t, len(lastErr), maxErrorLen)
	require.NotContains(t, lastErr, "\x00")
	require.True(t, strings.HasPrefix(lastErr, "x€"))
}

func TestErrTextBoundaries(t *testing.T) {
	require.Equal(t, "", errText(nil))
	s := errText(errors.New(strings.Repeat("€", 400)))
	require.Len(t, s, 999)
	require.Equal(t, 333, strings.Count(s, "€"))
}

func TestReclaimOfPoisonJobParks(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET status='running', attempts=4, claimed_at=now()-interval '11 minutes'`)
	got, err := q.Claim(ctx, "standard", 1)
	require.NoError(t, err)
	require.Empty(t, got, "poison job claimed")
	var status string
	var attempts int
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, attempts FROM ohs_sync.etl_jobs`).Scan(&status, &attempts))
	require.Equal(t, "parked", status)
	require.Equal(t, 5, attempts)
}

func TestPoisonParkDoesNotBlockOnLocksOrStarveClaim(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewETL(pool)
	enqueueETL(t, pool, etlJob(1, "standard"))
	enqueueETL(t, pool, etlJob(2, "standard"))
	mustExec(t, pool, `UPDATE ohs_sync.etl_jobs SET status='running', attempts=4, claimed_at=now()-interval '11 minutes' WHERE key='raw/k1'`)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT id FROM ohs_sync.etl_jobs WHERE key='raw/k1' FOR UPDATE`)
	require.NoError(t, err)

	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	got, err := q.Claim(cctx, "standard", 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "raw/k2", got[0].Key)
}

func pullJob(i int) PullJob {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour)
	return PullJob{TenantID: uuid.New(), UserID: uuid.New(), Provider: "withings", From: from, To: from.Add(time.Hour)}
}

func enqueuePull(t *testing.T, pool *pgxpool.Pool, j PullJob) {
	t.Helper()
	inTx(t, pool, func(tx pgx.Tx) error { return EnqueuePull(context.Background(), tx, j) })
}

func claimPull(t *testing.T, q *Pull) PullJob {
	t.Helper()
	got, err := q.Claim(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	return got[0]
}

func TestPullLifecycle(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewPull(pool)
	in := pullJob(1)
	enqueuePull(t, pool, in)
	got, err := q.Claim(ctx, 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	g := got[0]
	require.Equal(t, in.UserID, g.UserID)
	require.Equal(t, "withings", g.Provider)
	require.True(t, g.From.Equal(in.From))
	require.True(t, g.To.Equal(in.To))
	require.Zero(t, g.Attempts)
	require.False(t, g.ClaimedAt.IsZero())
	again, err := q.Claim(ctx, 5)
	require.NoError(t, err)
	require.Empty(t, again)
	require.NoError(t, q.Complete(ctx, g))
}

func TestPullEnqueueIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewPull(pool)
	j := pullJob(1)
	count := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM ohs_sync.pull_jobs`).Scan(&n))
		return n
	}
	enqueuePull(t, pool, j)
	enqueuePull(t, pool, j)
	require.Equal(t, 1, count())
	got := claimPull(t, q)
	enqueuePull(t, pool, j)
	require.Equal(t, 1, count(), "duplicate while running")
	require.NoError(t, q.Complete(ctx, got))
	enqueuePull(t, pool, j)
	require.Equal(t, 2, count(), "re-enqueue after done")
}

func TestPullNotBeforeHonoured(t *testing.T) {
	pool := testPool(t)
	q := NewPull(pool)
	j := pullJob(1)
	j.NotBefore = time.Now().Add(time.Hour)
	enqueuePull(t, pool, j)
	got, err := q.Claim(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDeferKeepsAttempts(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewPull(pool)
	enqueuePull(t, pool, pullJob(1))
	mustExec(t, pool, `UPDATE ohs_sync.pull_jobs SET attempts=2`)
	j := claimPull(t, q)
	until := time.Now().Add(15 * time.Minute)
	require.NoError(t, q.Defer(ctx, j, until))
	var status string
	var attempts int
	var nb time.Time
	var claimed *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, attempts, not_before, claimed_at FROM ohs_sync.pull_jobs`).Scan(&status, &attempts, &nb, &claimed))
	require.Equal(t, "pending", status)
	require.Equal(t, 2, attempts)
	require.Nil(t, claimed)
	require.WithinDuration(t, until, nb, time.Second)
	got, err := q.Claim(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, got, "claimable while deferred")
	mustExec(t, pool, `UPDATE ohs_sync.pull_jobs SET not_before=now()`)
	j = claimPull(t, q)
	require.Equal(t, 2, j.Attempts)
}

func TestPullStaleWorkerFenced(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewPull(pool)
	enqueuePull(t, pool, pullJob(1))
	a := claimPull(t, q)
	mustExec(t, pool, `UPDATE ohs_sync.pull_jobs SET claimed_at = now() - interval '11 minutes'`)
	b := claimPull(t, q)
	require.Equal(t, a.ID, b.ID)

	require.ErrorIs(t, q.Complete(ctx, a), ErrNotRunning)
	require.ErrorIs(t, q.Fail(ctx, a, errors.New("late")), ErrNotRunning)
	require.ErrorIs(t, q.Defer(ctx, a, time.Now().Add(time.Hour)), ErrNotRunning)

	var status string
	var attempts int
	var claimed time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, attempts, claimed_at FROM ohs_sync.pull_jobs`).Scan(&status, &attempts, &claimed))
	require.Equal(t, "running", status)
	require.Equal(t, b.Attempts, attempts)
	require.True(t, claimed.Equal(b.ClaimedAt))

	require.NoError(t, q.Complete(ctx, b))
	require.ErrorIs(t, q.Defer(ctx, b, time.Now()), ErrNotRunning, "done")
	require.ErrorIs(t, q.Complete(ctx, PullJob{ID: 424242, ClaimedAt: time.Now()}), ErrNotRunning, "unknown")
}

func TestPullFailAndPark(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewPull(pool)
	enqueuePull(t, pool, pullJob(1))
	for i := 1; i <= 5; i++ {
		mustExec(t, pool, `UPDATE ohs_sync.pull_jobs SET not_before = now() WHERE status='pending'`)
		require.NoError(t, q.Fail(ctx, claimPull(t, q), errors.New("x")))
	}
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM ohs_sync.pull_jobs`).Scan(&status))
	require.Equal(t, "parked", status)
}

func TestPullClaimSkipLocked(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewPull(pool)
	for i := 0; i < 10; i++ {
		enqueuePull(t, pool, pullJob(i))
	}
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT id FROM ohs_sync.pull_jobs ORDER BY id LIMIT 5 FOR UPDATE`)
	require.NoError(t, err)

	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	got, err := q.Claim(cctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 5)
	require.NoError(t, tx.Rollback(ctx))
	rest, err := q.Claim(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rest, 5)
}

func TestPullConcurrentClaimsDisjoint(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewPull(pool)
	for i := 0; i < 10; i++ {
		enqueuePull(t, pool, pullJob(i))
	}
	var wg sync.WaitGroup
	res := make([][]PullJob, 2)
	errs := make([]error, 2)
	for w := range res {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[w], errs[w] = q.Claim(ctx, 10)
		}()
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	seen := map[int64]bool{}
	for _, r := range res {
		for _, j := range r {
			require.False(t, seen[j.ID], "dup %d", j.ID)
			seen[j.ID] = true
		}
	}
	require.Len(t, seen, 10)
}
