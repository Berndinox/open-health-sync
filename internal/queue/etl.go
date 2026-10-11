// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ETL tiers.
const (
	TierStandard = "standard"
	TierHighFreq = "high_freq"
)

// ETLJob is one raw object waiting to be turned into canonical records.
type ETLJob struct {
	ID       int64
	TenantID uuid.UUID
	UserID   uuid.UUID
	Provider string
	Key      string
	Tier     string
	Attempts int
	// ClaimedAt is set by Claim and fences Complete and Fail.
	ClaimedAt time.Time
}

// EnqueueETL adds a job inside the caller's transaction, so it commits with
// the raw_objects row it belongs to. An empty Tier means standard.
func EnqueueETL(ctx context.Context, tx pgx.Tx, j ETLJob) error {
	if j.Tier == "" {
		j.Tier = TierStandard
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO ohs_sync.etl_jobs (tenant_id, user_id, provider, key, tier)
		VALUES ($1, $2, $3, $4, $5)`,
		j.TenantID, j.UserID, j.Provider, j.Key, j.Tier)
	if err != nil {
		return fmt.Errorf("enqueue etl job: %w", err)
	}
	return nil
}

// ETL claims and settles etl_jobs.
type ETL struct{ pool *pgxpool.Pool }

// NewETL returns the ETL queue over pool.
func NewETL(pool *pgxpool.Pool) *ETL { return &ETL{pool: pool} }

// Claim takes up to n jobs of one tier, oldest first. Pending jobs past their
// not_before and running jobs past the visibility timeout are eligible; a
// reclaimed job burns an attempt. Concurrent claimers get disjoint sets.
func (q *ETL) Claim(ctx context.Context, tier string, n int) ([]ETLJob, error) {
	if n <= 0 {
		return nil, nil
	}
	rows, err := q.pool.Query(ctx,
		claimSQL("etl_jobs", "tier = $3 AND ",
			"id, tenant_id, user_id, provider, key, tier, attempts, claimed_at"),
		n, VisibilityTimeout.Seconds(), tier)
	if err != nil {
		return nil, fmt.Errorf("claim etl jobs: %w", err)
	}
	defer rows.Close()
	var out []ETLJob
	for rows.Next() {
		var j ETLJob
		if err := rows.Scan(&j.ID, &j.TenantID, &j.UserID, &j.Provider, &j.Key, &j.Tier, &j.Attempts, &j.ClaimedAt); err != nil {
			return nil, fmt.Errorf("scan etl job: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim etl jobs: %w", err)
	}
	return out, nil
}

// Complete marks the job done. It returns ErrNotRunning if the claim is gone.
func (q *ETL) Complete(ctx context.Context, j ETLJob) error {
	return complete(ctx, "etl_jobs", q.pool, j.ID, j.ClaimedAt)
}

// Fail records the error and retries the job after 30s*2^attempts, or parks
// it once it has failed five times. It returns ErrNotRunning if the claim is gone.
func (q *ETL) Fail(ctx context.Context, j ETLJob, cause error) error {
	return fail(ctx, "etl_jobs", q.pool, j.ID, j.ClaimedAt, cause)
}
