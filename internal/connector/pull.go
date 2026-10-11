// SPDX-License-Identifier: AGPL-3.0-only

package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// DefaultRetryAfter is the wait used when a provider answers 429 without a
// usable Retry-After header.
const DefaultRetryAfter = time.Minute

// MaxRetryAfter caps the wait a provider can ask for.
const MaxRetryAfter = 24 * time.Hour

// RateLimitError is returned by Refresh or Fetch when the provider asked us to
// slow down (HTTP 429). The pull worker puts the job back until RetryAfter has
// passed without counting a failed attempt.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("connector: rate limited, retry after %s", e.RetryAfter)
}

// RateLimited builds a RateLimitError from a response's Retry-After header
// (delay seconds or an HTTP date), capped at MaxRetryAfter. A missing,
// invalid or past value gives DefaultRetryAfter.
func RateLimited(h http.Header, now time.Time) *RateLimitError {
	v := h.Get("Retry-After")
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil && secs > 0 {
		if secs > int64(MaxRetryAfter/time.Second) {
			return &RateLimitError{RetryAfter: MaxRetryAfter}
		}
		return &RateLimitError{RetryAfter: time.Duration(secs) * time.Second}
	}
	if at, err := http.ParseTime(v); err == nil && at.After(now) {
		return &RateLimitError{RetryAfter: min(at.Sub(now), MaxRetryAfter)}
	}
	return &RateLimitError{RetryAfter: DefaultRetryAfter}
}

// AsRateLimit reports whether err carries a provider rate limit.
func AsRateLimit(err error) (*RateLimitError, bool) {
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return rl, true
	}
	return nil, false
}

// Revoker is optionally implemented by an OAuthConnector whose provider can
// revoke a grant. Disconnecting calls it best effort.
type Revoker interface {
	Revoke(ctx context.Context, t Tokens) error
}
