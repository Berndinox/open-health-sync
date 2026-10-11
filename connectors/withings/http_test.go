// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

func TestJitter_SpreadsUpwardsAndVaries(t *testing.T) {
	const d = time.Second
	seen := map[time.Duration]bool{}
	for i := 0; i < 200; i++ {
		got := jitter(d)
		assert.GreaterOrEqual(t, got, d, "never sleeps less than asked")
		assert.Less(t, got, d+d/2+time.Nanosecond)
		seen[got] = true
	}
	assert.Greater(t, len(seen), 1, "a fixed schedule wakes every parallel fetch together")
	assert.Equal(t, time.Duration(0), jitter(0))
}

// noSleep records the transport retry waits instead of sleeping.
type noSleep struct{ waits []time.Duration }

func (n *noSleep) sleep(_ context.Context, d time.Duration) error {
	n.waits = append(n.waits, d)
	return nil
}

func testPoster(client *http.Client, sl *noSleep) *Connector {
	return New(Options{HTTPClient: client, RetryBase: 20 * time.Millisecond, Sleep: sl.sleep,
		Now: func() time.Time { return fakeNow }})
}

// A rate limit is returned at once as a *connector.RateLimitError: the pull
// worker defers the job, nothing sleeps.
func TestPost_RateLimitReturnsTypedErrorAtOnce(t *testing.T) {
	cases := []struct {
		name       string
		inner      bool
		retryAfter string
		want       time.Duration
	}{
		{"HTTP 429 with Retry-After", false, "120", 120 * time.Second},
		{"HTTP 429 without Retry-After", false, "", time.Minute},
		{"inner 601 with Retry-After", true, "30", 30 * time.Second},
		{"inner 601 without Retry-After", true, "", time.Minute},
		{"HTTP date", false, fakeNow.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if c.retryAfter != "" {
					w.Header().Set("Retry-After", c.retryAfter)
				}
				if c.inner {
					_, _ = w.Write([]byte(`{"status":601}`))
					return
				}
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer srv.Close()
			sl := &noSleep{}
			var out envelope
			err := testPoster(srv.Client(), sl).post(context.Background(), "tok", srv.URL, url.Values{}, &out)
			rl, ok := connector.AsRateLimit(err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, c.want, rl.RetryAfter)
			assert.Equal(t, int32(1), calls.Load(), "no retry inside the connector")
			assert.Empty(t, sl.waits, "no sleep")
		})
	}
}

type stubRoundTripper struct {
	calls atomic.Int32
	fn    func(n int32, r *http.Request) (*http.Response, error)
}

func (s *stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return s.fn(s.calls.Add(1), r)
}

// A dropped connection is retried rather than failing the window.
func TestPost_RetriesTransportError(t *testing.T) {
	rt := &stubRoundTripper{}
	rt.fn = func(n int32, r *http.Request) (*http.Response, error) {
		if n == 1 {
			return nil, errors.New("connection reset by peer")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{},
			Body: io.NopCloser(strings.NewReader(`{"status":0}`)), Request: r}, nil
	}
	var out envelope
	sl := &noSleep{}
	require.NoError(t, testPoster(&http.Client{Transport: rt}, sl).post(context.Background(), "tok", "http://withings.invalid/measure", url.Values{}, &out))
	assert.Equal(t, int32(2), rt.calls.Load())
	require.Len(t, sl.waits, 1)
	assert.GreaterOrEqual(t, sl.waits[0], 20*time.Millisecond)
}

func TestPost_TransportErrorSurfacesAfterTheBudget(t *testing.T) {
	rt := &stubRoundTripper{}
	rt.fn = func(int32, *http.Request) (*http.Response, error) { return nil, errors.New("connection reset by peer") }
	var out envelope
	sl := &noSleep{}
	err := testPoster(&http.Client{Transport: rt}, sl).post(context.Background(), "tok", "http://withings.invalid/measure", url.Values{}, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "withings http")
	assert.Equal(t, int32(5), rt.calls.Load())
	assert.Len(t, sl.waits, 4, "no pause after the last attempt")
	rung := 20 * time.Millisecond
	for i, w := range sl.waits {
		assert.GreaterOrEqual(t, w, rung, "rung %d", i)
		assert.Less(t, w, rung+rung/2+time.Nanosecond, "rung %d", i)
		rung *= 2
	}
}

func TestPost_401IsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	var out envelope
	err := testPoster(srv.Client(), &noSleep{}).post(context.Background(), "tok", srv.URL, url.Values{}, &out)
	assert.ErrorIs(t, err, errUnauthorized)
}

func TestNewHTTPClient_PoolsConnectionsPerHost(t *testing.T) {
	tr, ok := New(Options{}).httpc.Transport.(*http.Transport)
	require.True(t, ok, "an explicit transport, not http.DefaultTransport")
	assert.Equal(t, 100, tr.MaxIdleConnsPerHost)
	assert.Equal(t, 100, tr.MaxIdleConns)
	assert.Equal(t, 90*time.Second, tr.IdleConnTimeout)
	assert.Equal(t, 10*time.Second, tr.TLSHandshakeTimeout)
}

func TestReadBody_RefusesPastTheCap(t *testing.T) {
	body, err := readBody(strings.NewReader("0123456789"), 10)
	require.NoError(t, err)
	assert.Len(t, body, 10)
	_, err = readBody(strings.NewReader("0123456789X"), 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestIntOrBool(t *testing.T) {
	for in, want := range map[string]IntOrBool{`true`: 1, `false`: 0, `1`: 1, `0`: 0, `7`: 7} {
		var got IntOrBool
		require.NoError(t, json.Unmarshal([]byte(in), &got), in)
		assert.Equal(t, want, got, in)
	}
	var bad IntOrBool
	assert.Error(t, json.Unmarshal([]byte(`"yes"`), &bad), "a string is a decode error, not a silent zero")

	var a, b struct {
		More IntOrBool `json:"more"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"more":true}`), &a))
	require.NoError(t, json.Unmarshal([]byte(`{"more":1}`), &b))
	assert.Equal(t, a.More, b.More)
}

func TestFlexString(t *testing.T) {
	var s FlexString
	require.NoError(t, json.Unmarshal([]byte(`"abc"`), &s))
	assert.Equal(t, FlexString("abc"), s)
	require.NoError(t, json.Unmarshal([]byte(`98765432`), &s))
	assert.Equal(t, FlexString("98765432"), s)
	require.NoError(t, json.Unmarshal([]byte(`null`), &s))
	assert.Equal(t, FlexString(""), s)
	assert.Error(t, json.Unmarshal([]byte(`{}`), &s))
}
