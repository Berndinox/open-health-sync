// SPDX-License-Identifier: AGPL-3.0-only

package collect

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/connector"
	"github.com/soma-intelligence/open-health-sync/internal/queue"
	"github.com/soma-intelligence/open-health-sync/internal/rawstore"
)

// pullSink stores what one pull job fetched. Every Emit is one raw object of
// one data type (R16): key raw/<tenant>/<user>/<provider>/<day>/<batch>_<part>
// with batch naming the job and its claim and part the emit sequence. The
// raw_objects row and its standard tier ETL job are inserted in one
// transaction (R18). Bytes already registered for this user (an overlapping
// pull) are skipped, so a re-pull of the same data stores nothing new.
type pullSink struct {
	d     Deps
	job   queue.PullJob
	batch string

	mu      sync.Mutex
	part    int
	stored  int
	skipped int
}

var _ connector.RawSink = (*pullSink)(nil)

func newPullSink(d Deps, job queue.PullJob) *pullSink {
	// The claim time keeps keys of a retried or deferred job apart from the
	// files an earlier attempt already wrote (raw files are never rewritten).
	return &pullSink{d: d, job: job, batch: fmt.Sprintf("pull%d-%d", job.ID, job.ClaimedAt.UnixMicro())}
}

// Emit stores lines (NDJSON, one record each, verbatim) as one raw object.
func (s *pullSink) Emit(ctx context.Context, dataType string, day time.Time, lines [][]byte) error {
	if dataType == "" {
		return errors.New("collect: emit without a data type")
	}
	if len(lines) == 0 {
		return nil
	}
	body, err := gzipLines(lines)
	if err != nil {
		return err
	}

	s.mu.Lock()
	part := s.part
	s.part++
	s.mu.Unlock()

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	j := s.job
	key := rawstore.Key(j.TenantID, j.UserID, j.Provider, day, s.batch, part)

	if _, found, err := findRaw(ctx, s.d.Pool, j.TenantID, j.UserID, key, hash); err != nil {
		return err
	} else if found {
		s.count(false)
		return nil
	}
	if err := userActive(ctx, s.d.Pool, j.TenantID, j.UserID); err != nil {
		return err
	}
	if err := putRaw(ctx, s.d.Raw, key, body); err != nil {
		if errors.Is(err, errOtherBytes) {
			return fmt.Errorf("collect: raw key %s already holds other bytes", key)
		}
		return err
	}
	inserted, err := registerRaw(ctx, s.d.Pool, rawObject{
		tenant: j.TenantID, user: j.UserID, provider: j.Provider, key: key, hash: hash, size: len(body),
		batch: s.batch, part: part, dataType: dataType, tier: queue.TierStandard,
	})
	if errors.Is(err, errInactive) {
		// The user started being deleted between the check and the
		// registration: the file just written is removed again, or it
		// could outlive the deletion's own raw sweep.
		if derr := s.d.Raw.DeletePrefix(context.WithoutCancel(ctx), key); derr != nil {
			slog.WarnContext(ctx, "collect: could not remove the file of a refused pull",
				"tenant_id", j.TenantID, "user_id", j.UserID, "provider", j.Provider, "error", derr)
		}
		return errInactive
	}
	if err != nil {
		return err
	}
	// Not inserted: a concurrent pull registered the same bytes first. The
	// file just written is then an orphan the daily scan reports.
	s.count(inserted)
	return nil
}

func (s *pullSink) count(stored bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stored {
		s.stored++
	} else {
		s.skipped++
	}
}

func gzipLines(lines [][]byte) ([]byte, error) {
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	for _, l := range lines {
		l = bytes.TrimRight(l, "\r\n")
		if bytes.ContainsAny(l, "\n") {
			return nil, errors.New("collect: emitted line contains a newline")
		}
		if _, err := zw.Write(l); err != nil {
			return nil, err
		}
		if _, err := zw.Write([]byte{'\n'}); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
