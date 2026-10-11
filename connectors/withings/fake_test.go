// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/soma-intelligence/open-health-sync/connectors/withings/withingsfake"
)

// fakeWithings wraps the httptest Withings of withingsfake.
type fakeWithings struct {
	*withingsfake.Server
	// slept records the waits of transport retries (no real sleep).
	mu    sync.Mutex
	slept []time.Duration
}

func newFakeWithings(t *testing.T) *fakeWithings {
	t.Helper()
	return &fakeWithings{Server: withingsfake.New(t)}
}

var fakeNow = time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)

// connector returns a Withings connector pointed at the fake, with test keys,
// no pacing, a fixed clock and a recording sleeper.
func (f *fakeWithings) connector() *Connector {
	return New(Options{
		ClientID: "test-client-id", ClientSecret: "test-client-secret",
		AccountBase: f.URL(), APIBase: f.URL(),
		HTTPClient: f.Client(),
		ChunkDelay: -1, PageDelay: -1,
		Now: func() time.Time { return fakeNow },
		Sleep: func(_ context.Context, d time.Duration) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.slept = append(f.slept, d)
			return nil
		},
	})
}

func (f *fakeWithings) calls(path, action string) []withingsfake.Request {
	return f.Calls(path, action)
}

// recordingSink is a connector.RawSink that keeps every Emit call.
type recordingSink struct {
	mu    sync.Mutex
	calls []emitCall
}

type emitCall struct {
	DataType string
	Day      time.Time
	Lines    [][]byte
}

func (s *recordingSink) Emit(_ context.Context, dataType string, day time.Time, lines [][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([][]byte, len(lines))
	for i, l := range lines {
		cp[i] = append([]byte(nil), l...)
	}
	s.calls = append(s.calls, emitCall{DataType: dataType, Day: day, Lines: cp})
	return nil
}

func (s *recordingSink) linesOf(dataType string) [][]byte {
	var out [][]byte
	for _, c := range s.calls {
		if c.DataType == dataType {
			out = append(out, c.Lines...)
		}
	}
	return out
}
