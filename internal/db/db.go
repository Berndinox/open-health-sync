// SPDX-License-Identifier: AGPL-3.0-only

// Package db opens the Postgres pool and applies the embedded schema migrations.
package db

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open connects to url and checks the connection with a ping.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// ConnURL is dbURL without the pgxpool settings (pool_max_conns and the other
// pool_ parameters). URL form only (postgres://...). Those are read by the pool only; a single connection
// (pgx.Connect, the migrator) would send them to the server as run-time
// parameters, and Postgres refuses unknown ones. A URL that does not parse,
// or a keyword/value string, is returned unchanged.
func ConnURL(dbURL string) string {
	u, err := url.Parse(dbURL)
	if err != nil || u.Scheme == "" || u.RawQuery == "" {
		return dbURL
	}
	q := u.Query()
	changed := false
	for k := range q {
		if strings.HasPrefix(k, "pool_") {
			q.Del(k)
			changed = true
		}
	}
	if !changed {
		return dbURL
	}
	u.RawQuery = q.Encode()
	return u.String()
}
