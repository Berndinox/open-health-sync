// SPDX-License-Identifier: AGPL-3.0-only

package read

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// QuantityType is one canonical quantity type the server knows: the value of
// the type parameter of Samples, with the unit its samples are stored in.
type QuantityType struct {
	Type      string  `json:"type"`
	Unit      *string `json:"unit"`
	UCUMUnit  *string `json:"ucum_unit"`
	LOINCCode *string `json:"loinc_code"`
}

// QuantityTypes lists the quantity type catalog (ohs_data.quantity_codes, a
// reference table shared by every tenant), ordered by type.
func QuantityTypes(ctx context.Context, db Querier) ([]QuantityType, error) {
	rows, err := db.Query(ctx, `SELECT canonical_code, storage_unit, ucum_unit, loinc_code
		FROM ohs_data.quantity_codes ORDER BY canonical_code`)
	if err != nil {
		return nil, fmt.Errorf("read: quantity types: %w", err)
	}
	defer rows.Close()
	out := []QuantityType{}
	for rows.Next() {
		var q QuantityType
		if err := rows.Scan(&q.Type, &q.Unit, &q.UCUMUnit, &q.LOINCCode); err != nil {
			return nil, fmt.Errorf("read: quantity types: scan: %w", err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read: quantity types: %w", err)
	}
	return out, nil
}

// Connection is a user's link to one provider. Tokens never leave the store.
type Connection struct {
	Provider    string     `json:"provider"`
	Status      string     `json:"status"` // active, needs_reauth or disconnected
	LastSyncAt  *time.Time `json:"last_sync_at"`
	ConnectedAt time.Time  `json:"connected_at"`
}

// Connections lists every provider connection of the user, ordered by
// provider. ErrNotFound when the user is not the tenant's.
func Connections(ctx context.Context, db Querier, tenantID, userID uuid.UUID) ([]Connection, error) {
	if err := userExists(ctx, db, tenantID, userID); err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT provider, status, last_sync_at, created_at
		FROM ohs_sync.connections WHERE tenant_id = $1 AND user_id = $2 ORDER BY provider`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("read: connections: %w", err)
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.Provider, &c.Status, &c.LastSyncAt, &c.ConnectedAt); err != nil {
			return nil, fmt.Errorf("read: connections: scan: %w", err)
		}
		utc(c.LastSyncAt)
		utc(&c.ConnectedAt)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read: connections: %w", err)
	}
	return out, nil
}
