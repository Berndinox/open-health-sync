-- SPDX-License-Identifier: AGPL-3.0-only
-- The scheduler looks for the job of one connection ending at a slot boundary.
CREATE INDEX pull_jobs_slot ON ohs_sync.pull_jobs (tenant_id, user_id, provider, to_time);
