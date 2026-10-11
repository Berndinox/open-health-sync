// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"net/http"

	"github.com/soma-intelligence/open-health-sync/internal/auth"
	"github.com/soma-intelligence/open-health-sync/internal/collect"
)

// ingestRetryAfter is the Retry-After (seconds) of a refused upload.
const ingestRetryAfter = "5"

func (s *server) mountIngest(mux *http.ServeMux) {
	if s.d.Device == nil {
		return
	}
	n := s.d.IngestLimit
	if n <= 0 {
		n = DefaultIngestLimit
	}
	upload := collect.NewUploadHandler(collect.UploadDeps{Pool: s.d.Pool, Raw: s.d.Raw, Device: s.d.Device})
	// Authenticate first, so an anonymous caller gets 401 and never a slot.
	mux.Handle("POST /v1/ingest/apple-health",
		auth.Middleware(s.lookup, auth.UserUpload)(limitInFlight(n, s.metrics, upload)))
}

// limitInFlight lets at most n requests through at once. One upload can hold
// 64 MiB of body plus its decompression buffers, so the limit bounds memory.
// A request over the limit is refused at once with 503 and Retry-After; it is
// never queued.
func limitInFlight(n int, m *httpMetrics, next http.Handler) http.Handler {
	slots := make(chan struct{}, n)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
		default:
			m.ingestRejected.Add(1)
			w.Header().Set("Retry-After", ingestRetryAfter)
			problem(w, http.StatusServiceUnavailable, "too many uploads in progress, retry later")
			return
		}
		m.ingestInFlight.Add(1)
		defer func() {
			m.ingestInFlight.Add(-1)
			<-slots
		}()
		next.ServeHTTP(w, r)
	})
}
