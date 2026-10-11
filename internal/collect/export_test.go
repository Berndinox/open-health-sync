// SPDX-License-Identifier: AGPL-3.0-only

package collect

import "time"

// SetInboundReadTimeout shortens the notification body deadline for a test
// and returns the restore function.
func SetInboundReadTimeout(d time.Duration) (restore func()) {
	old := inboundReadTimeout
	inboundReadTimeout = d
	return func() { inboundReadTimeout = old }
}

// SetRevokeTimeout shortens the provider budget of a disconnect or a
// RevokeAll for a test and returns the restore function.
func SetRevokeTimeout(d time.Duration) (restore func()) {
	old := revokeTimeout
	revokeTimeout = d
	return func() { revokeTimeout = old }
}
