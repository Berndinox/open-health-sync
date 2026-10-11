// SPDX-License-Identifier: AGPL-3.0-only

package connector

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRateLimited(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"120":                           120 * time.Second,
		"":                              DefaultRetryAfter,
		"soon":                          DefaultRetryAfter,
		"-5":                            DefaultRetryAfter,
		"Sat, 10 Oct 2026 12:05:00 GMT": 5 * time.Minute,
		"Sat, 10 Oct 2026 11:00:00 GMT": DefaultRetryAfter,
		"86401":                         MaxRetryAfter,
		"99999999999999999":             MaxRetryAfter,
		"Fri, 01 Jan 2100 00:00:00 GMT": MaxRetryAfter,
	}
	for v, want := range cases {
		h := http.Header{}
		if v != "" {
			h.Set("Retry-After", v)
		}
		if got := RateLimited(h, now).RetryAfter; got != want {
			t.Errorf("Retry-After %q: got %s, want %s", v, got, want)
		}
	}
	wrapped := fmt.Errorf("fetch: %w", RateLimited(http.Header{"Retry-After": {"7"}}, now))
	rl, ok := AsRateLimit(wrapped)
	if !ok || rl.RetryAfter != 7*time.Second {
		t.Fatalf("AsRateLimit(wrapped) = %v, %v", rl, ok)
	}
	if _, ok := AsRateLimit(errors.New("other")); ok {
		t.Fatal("plain error reported as a rate limit")
	}
}
