// SPDX-License-Identifier: AGPL-3.0-only

package connector

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/soma-intelligence/open-health-sync/internal/config"
)

// Config is the set of keys one provider was configured with
// (OHS_<PROVIDER>_<FIELD> environment variables, keyed by FIELD).
type Config = config.ProviderKeys

// ErrNeedsReauth is returned by Refresh and Fetch when the provider rejects
// the grant and the user has to connect again.
var ErrNeedsReauth = errors.New("connector: provider rejected the grant, reauthorization needed")

// RawRecord is one raw payload handed to Connector.Map.
type RawRecord struct {
	TenantID  string
	UserID    string
	Provider  string
	DataType  string
	SourceKey string
	Fields    map[string]any
}

// RawSink receives raw NDJSON lines a connector fetched or parsed. The
// implementation stores them under the raw key layout.
type RawSink interface {
	Emit(ctx context.Context, dataType string, day time.Time, lines [][]byte) error
}

// Tokens are the OAuth credentials of one connection.
type Tokens struct {
	Access         string
	Refresh        string
	Expiry         time.Time
	Scopes         []string
	ProviderUserID string
}

// Connection is one user's link to one provider.
type Connection struct {
	TenantID   uuid.UUID
	UserID     uuid.UUID
	Provider   string
	Tokens     Tokens
	LastSyncAt time.Time
}

// InboundEvent is a provider notification that new data is available.
type InboundEvent struct {
	ProviderUserID string
	DataType       string
	From, To       time.Time
}

// UploadMeta describes one uploaded device part.
type UploadMeta struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	Type     string
	Day      time.Time
	Batch    string
	Part     int
}
