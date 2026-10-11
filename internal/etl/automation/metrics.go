// SPDX-License-Identifier: AGPL-3.0-only

package automation

import "sync/atomic"

// Metrics holds the counters and gauges of the automation jobs. It has no
// dependency on a metrics library: the process exposes Snapshot however it
// likes (Task 12 wires /metrics).
//
// Gauges are overwritten by each run, counters only grow.
type Metrics struct {
	// Gauges.
	PendingFailedRecords atomic.Int64 // failed_records still pending (alert if > 0)
	ParkedJobs           atomic.Int64 // etl_jobs that exhausted their attempts
	UnprocessedObjects   atomic.Int64 // raw objects past the grace period with no finished job
	UnregisteredFiles    atomic.Int64 // raw files in the store with no raw_objects row

	// Counters.
	OrphansEnqueued   atomic.Int64 // jobs created for raw_objects rows that had none
	DrainerReplays    atomic.Int64 // raw objects re-enqueued by the drainer
	RetentionRows     atomic.Int64 // ohs_data rows deleted by retention
	RetentionObjects  atomic.Int64 // raw day prefixes deleted by retention
	RunFailures       atomic.Int64 // job runs that returned an error
	LastReconcileUnix atomic.Int64 // when the last reconcile pass finished
}

// Snapshot returns the current values keyed by metric name.
func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"pending_failed_records":      m.PendingFailedRecords.Load(),
		"parked_jobs":                 m.ParkedJobs.Load(),
		"unprocessed_objects":         m.UnprocessedObjects.Load(),
		"unregistered_files":          m.UnregisteredFiles.Load(),
		"orphans_enqueued_total":      m.OrphansEnqueued.Load(),
		"drainer_replays_total":       m.DrainerReplays.Load(),
		"retention_rows_total":        m.RetentionRows.Load(),
		"retention_prefixes_total":    m.RetentionObjects.Load(),
		"run_failures_total":          m.RunFailures.Load(),
		"last_reconcile_unix_seconds": m.LastReconcileUnix.Load(),
	}
}
