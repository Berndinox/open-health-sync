// SPDX-License-Identifier: AGPL-3.0-only

package etl

import (
	"testing"

	"github.com/soma-intelligence/open-health-sync/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pruneRejected must leave the MapResult untouched when it cannot scope the
// rejection, so the caller's whole-record drop is the only outcome.
func TestPruneRejected_LeavesRecordIntactOnUnknownRecordType(t *testing.T) {
	t.Parallel()

	mr := &model.MapResult{
		QuantitySamples: []model.QuantitySample{{QuantityType: "heart_rate", Value: 72}},
	}
	structural := pruneRejected(mr, []ValidationError{
		{RecordType: "something_new", Field: "value", Index: 0, Severity: SeverityReject},
	})

	require.Len(t, structural, 1, "an unknown record type must fail safe as structural")
	assert.Len(t, mr.QuantitySamples, 1, "nothing may be pruned when the reject cannot be scoped")
}

// Several rejects in one record, across two families, all get scoped.
func TestPruneRejected_ScopesAcrossFamilies(t *testing.T) {
	t.Parallel()

	mr := &model.MapResult{
		QuantitySamples: []model.QuantitySample{
			{QuantityType: "heart_rate", Value: 60},
			{QuantityType: "heart_rate", Value: 999},
			{QuantityType: "heart_rate", Value: 70},
		},
		HeartbeatSamples: []model.HeartbeatSample{
			{RRMS: 800}, {RRMS: -1},
		},
	}
	structural := pruneRejected(mr, []ValidationError{
		{RecordType: "quantity_sample", Index: 1, Severity: SeverityReject},
		{RecordType: "heartbeat_sample", Index: 1, Severity: SeverityReject},
	})

	assert.Empty(t, structural)
	require.Len(t, mr.QuantitySamples, 2)
	assert.Equal(t, 60.0, mr.QuantitySamples[0].Value)
	assert.Equal(t, 70.0, mr.QuantitySamples[1].Value)
	require.Len(t, mr.HeartbeatSamples, 1)
	assert.Equal(t, 800.0, mr.HeartbeatSamples[0].RRMS)
}
