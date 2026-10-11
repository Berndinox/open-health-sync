// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package etl

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/connector"
	"github.com/soma-intelligence/open-health-sync/internal/db"
	"github.com/soma-intelligence/open-health-sync/internal/model"
	"github.com/soma-intelligence/open-health-sync/internal/queue"
	"github.com/soma-intelligence/open-health-sync/internal/rawstore"
)

// env is one migrated database, one raw store and one tenant with one active
// user, which is what nearly every ETL test needs.
type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	rs     rawstore.Store
	tenant uuid.UUID
	user   uuid.UUID
	reg    *connector.Registry
	now    func() time.Time // validation clock; nil means time.Now
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool, err := db.Open(context.Background(), db.TestDB(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	rs, err := rawstore.Open("file://" + t.TempDir())
	require.NoError(t, err)
	e := &env{t: t, pool: pool, rs: rs, reg: connector.NewRegistry()}
	e.tenant = e.newTenant()
	e.user = e.newUser(e.tenant)
	return e
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	_, err := e.pool.Exec(context.Background(), sql, args...)
	require.NoError(e.t, err)
}

func (e *env) newTenant() uuid.UUID {
	e.t.Helper()
	var id uuid.UUID
	require.NoError(e.t, e.pool.QueryRow(context.Background(),
		`INSERT INTO ohs_core.tenants (name) VALUES ('t') RETURNING id`).Scan(&id))
	return id
}

func (e *env) newUser(tenant uuid.UUID) uuid.UUID {
	e.t.Helper()
	var id uuid.UUID
	require.NoError(e.t, e.pool.QueryRow(context.Background(),
		`INSERT INTO ohs_core.end_users (tenant_id) VALUES ($1) RETURNING id`, tenant).Scan(&id))
	return id
}

// count returns the number of rows of table for the env's tenant and user.
func (e *env) count(table string) int {
	e.t.Helper()
	return e.countFor(table, e.tenant, e.user)
}

func (e *env) countFor(table string, tenant, user uuid.UUID) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM ohs_data.%s WHERE tenant_id = $1 AND user_id = $2`, pgx.Identifier{table}.Sanitize()),
		tenant, user).Scan(&n))
	return n
}

var batchSeq atomic.Int64

// upload stores lines as one gzipped raw object of dataType for (tenant, user,
// provider), registers it in raw_objects and returns the job that processes it.
func (e *env) upload(tenant, user uuid.UUID, provider, dataType string, lines ...string) queue.ETLJob {
	e.t.Helper()
	ctx := context.Background()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	for _, l := range lines {
		_, _ = gz.Write([]byte(l + "\n"))
	}
	require.NoError(e.t, gz.Close())
	body := buf.Bytes()
	batch := fmt.Sprintf("b%d", batchSeq.Add(1))
	key := rawstore.Key(tenant, user, provider, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), batch, 0)
	require.NoError(e.t, e.rs.Put(ctx, key, bytes.NewReader(body)))
	sum := sha256.Sum256(append([]byte(batch), body...))
	_, err := e.pool.Exec(ctx, `
		INSERT INTO ohs_sync.raw_objects (tenant_id, user_id, provider, key, content_hash, size_bytes, batch, part, data_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $8)`,
		tenant, user, provider, key, hex.EncodeToString(sum[:]), len(body), batch, dataType)
	require.NoError(e.t, err)
	return queue.ETLJob{TenantID: tenant, UserID: user, Provider: provider, Key: key, Tier: queue.TierStandard}
}

// register adds a fake connector to the env's private registry.
func (e *env) register(slug string, mapFn func(connector.RawRecord) (model.MapResult, error)) {
	e.reg.Register(&fakeConnector{slug: slug, mapFn: mapFn})
}

func (e *env) opts() Options { return Options{Lookup: e.reg.Get, Now: e.now} }

func (e *env) process(job queue.ETLJob) Counts {
	e.t.Helper()
	c, err := ProcessWith(context.Background(), e.pool, e.rs, job, e.opts())
	require.NoError(e.t, err)
	return c
}

// fakeConnector maps with a test supplied function.
type fakeConnector struct {
	slug  string
	mapFn func(connector.RawRecord) (model.MapResult, error)
}

func (f *fakeConnector) Slug() string                  { return f.slug }
func (f *fakeConnector) Enabled(connector.Config) bool { return true }
func (f *fakeConnector) Map(r connector.RawRecord) (model.MapResult, error) {
	return f.mapFn(r)
}

// line marshals one raw NDJSON line.
func line(fields map[string]any) string {
	b, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// quantityMap is the mapper most tests use: one line with type, value, unit,
// start, end and an optional tz becomes one quantity sample whose content hash
// covers its fields. A line with "fail" set is a mapping error; a line with
// "empty" set maps to nothing.
func quantityMap(r connector.RawRecord) (model.MapResult, error) {
	if msg, ok := r.Fields["fail"].(string); ok {
		return model.MapResult{}, fmt.Errorf("%s", msg)
	}
	if _, ok := r.Fields["empty"]; ok {
		return model.MapResult{}, nil
	}
	start, err := time.Parse(time.RFC3339, r.Fields["start"].(string))
	if err != nil {
		return model.MapResult{}, err
	}
	end := start
	if s, ok := r.Fields["end"].(string); ok {
		if end, err = time.Parse(time.RFC3339, s); err != nil {
			return model.MapResult{}, err
		}
	}
	qt := r.Fields["type"].(string)
	v := r.Fields["value"].(float64)
	unit, _ := r.Fields["unit"].(string)
	md := map[string]any{}
	src, _ := r.Fields["source"].(string)
	if tz, ok := r.Fields["tz"].(string); ok {
		md["timezone"] = tz
	}
	s := model.QuantitySample{
		TenantID:     r.TenantID,
		UserID:       r.UserID,
		Provider:     r.Provider,
		QuantityType: qt,
		Value:        v,
		Unit:         unit,
		StartDate:    start.UTC(),
		EndDate:      end.UTC(),
		SourceName:   src,
		Metadata:     md,
		ContentHash: model.ComputeContentHash(map[string]string{
			"user_id": r.UserID, "type": qt, "start": start.UTC().Format(time.RFC3339Nano),
			"value": model.CanonicalFloat(v), "source": src,
		}),
	}
	if rep, ok := r.Fields["replaceable"].(bool); ok && rep {
		s.Replaceable = true
		s.ContentHash = model.ComputeContentHash(map[string]string{
			"user_id": r.UserID, "type": qt, "start": start.UTC().Format(time.RFC3339Nano),
		})
	}
	return model.MapResult{QuantitySamples: []model.QuantitySample{s}}, nil
}

func q(qt string, v float64, unit string, start time.Time) string {
	return line(map[string]any{"type": qt, "value": v, "unit": unit, "start": start.UTC().Format(time.RFC3339)})
}
