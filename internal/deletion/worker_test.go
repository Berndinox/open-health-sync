// SPDX-License-Identifier: AGPL-3.0-only

package deletion

import (
	"testing"
	"time"
)

func TestRevokeBudgetScalesPerConnection(t *testing.T) {
	for n, want := range map[int]time.Duration{
		0:  35 * time.Second,
		1:  35 * time.Second,
		3:  105 * time.Second,
		8:  280 * time.Second,
		9:  5 * time.Minute,
		50: 5 * time.Minute,
	} {
		if got := revokeBudget(n); got != want {
			t.Errorf("revokeBudget(%d) = %s, want %s", n, got, want)
		}
	}
}
