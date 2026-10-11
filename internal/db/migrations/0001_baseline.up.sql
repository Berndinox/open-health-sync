-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Open Health Sync baseline schema. Curated by hand from the final schema of the code Open Health Sync was
-- ported from (see scripts/build-baseline.sh). Three schemas:
--   ohs_core  tenants, keys, end users, user tokens, idempotency, deletion jobs
--   ohs_sync  provider connections, OAuth state, job queues, coverage, logs, raw objects
--   ohs_data  canonical health records (TimescaleDB hypertables) and reference tables
--
-- Conventions:
--   * Every row of ohs_sync and ohs_data carries tenant_id and user_id (uuid NOT NULL),
--     except the two reference tables ohs_data.quantity_codes and ohs_data.device_aliases.
--   * Dedup: canonical records are unique on (tenant_id, user_id, content_hash) plus the
--     time partition column on hypertables (TimescaleDB requires it in every unique index).
--   * Indexes that led with user_id lead with (tenant_id, user_id).
--   * Only hypertables. No continuous aggregates, no compression policies.

CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE SCHEMA ohs_core;
CREATE SCHEMA ohs_sync;
CREATE SCHEMA ohs_data;

-- ===========================================================================
-- ohs_core
-- ===========================================================================

CREATE TABLE ohs_core.tenants (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Secret keys are stored as SHA-256 hex only. key_prefix keeps the first
-- characters for display in listings.
CREATE TABLE ohs_core.api_keys (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES ohs_core.tenants (id) ON DELETE CASCADE,
    key_prefix   text        NOT NULL,
    key_hash     text        NOT NULL,
    last_used_at timestamptz,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT api_keys_key_hash_key UNIQUE (key_hash)
);
CREATE INDEX api_keys_tenant ON ohs_core.api_keys (tenant_id);

CREATE TABLE ohs_core.end_users (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES ohs_core.tenants (id) ON DELETE CASCADE,
    external_ref text,
    status       text        NOT NULL DEFAULT 'active',
    metadata     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT end_users_status_check CHECK (status IN ('active', 'deleting')),
    CONSTRAINT end_users_tenant_external_ref_key UNIQUE (tenant_id, external_ref),
    CONSTRAINT end_users_tenant_id_key UNIQUE (tenant_id, id)
);
CREATE INDEX end_users_tenant_created ON ohs_core.end_users (tenant_id, created_at, id);

-- Opaque user tokens (ohs_ut_...), stored as SHA-256 hex only.
CREATE TABLE ohs_core.user_tokens (
    hash       text        PRIMARY KEY,
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    scopes     text[]      NOT NULL DEFAULT '{}',
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_tokens_user_fk FOREIGN KEY (tenant_id, user_id)
        REFERENCES ohs_core.end_users (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX user_tokens_user ON ohs_core.user_tokens (tenant_id, user_id);
CREATE INDEX user_tokens_expires ON ohs_core.user_tokens (expires_at);

CREATE TABLE ohs_core.idempotency_keys (
    tenant_id     uuid        NOT NULL REFERENCES ohs_core.tenants (id) ON DELETE CASCADE,
    key           text        NOT NULL,
    request_hash  text        NOT NULL,
    status_code   integer     NOT NULL,
    response_body bytea       NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key)
);
CREATE INDEX idempotency_keys_created ON ohs_core.idempotency_keys (created_at);

-- Deletion jobs outlive the user they delete, so no foreign key to end_users.
CREATE TABLE ohs_core.deletion_jobs (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL,
    user_id     uuid        NOT NULL,
    status      text        NOT NULL DEFAULT 'pending',
    created_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    CONSTRAINT deletion_jobs_status_check CHECK (status IN ('pending', 'running', 'done', 'failed'))
);
CREATE INDEX deletion_jobs_pending ON ohs_core.deletion_jobs (created_at) WHERE status IN ('pending', 'running');
CREATE INDEX deletion_jobs_user ON ohs_core.deletion_jobs (tenant_id, user_id);

-- ===========================================================================
-- ohs_sync
-- ===========================================================================

-- One connection per user and provider. OAuth tokens are AES-256-GCM sealed.
CREATE TABLE ohs_sync.connections (
    tenant_id           uuid        NOT NULL,
    user_id             uuid        NOT NULL,
    provider            text        NOT NULL,
    status              text        NOT NULL DEFAULT 'active',
    provider_user_id    text,
    access_token_enc    bytea,
    refresh_token_enc   bytea,
    token_expires_at    timestamptz,
    token_type          text,
    scopes              text[]      NOT NULL DEFAULT '{}',
    last_sync_at        timestamptz,
    initial_backfill_at timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, provider),
    CONSTRAINT connections_status_check CHECK (status IN ('active', 'needs_reauth', 'disconnected'))
);
-- Inbound notifications name the provider's user id: find the connection from it.
CREATE INDEX connections_provider_user ON ohs_sync.connections (provider, provider_user_id) WHERE status = 'active';
CREATE INDEX connections_needs_backfill ON ohs_sync.connections (provider) WHERE status = 'active' AND initial_backfill_at IS NULL;

-- Single-use OAuth state, 10 minute lifetime.
CREATE TABLE ohs_sync.oauth_states (
    state        text        PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    user_id      uuid        NOT NULL,
    provider     text        NOT NULL,
    redirect_uri text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);
CREATE INDEX oauth_states_expires ON ohs_sync.oauth_states (expires_at);
CREATE INDEX oauth_states_user ON ohs_sync.oauth_states (tenant_id, user_id);

-- Pull queue: one job pulls one provider for one user over [from_time, to_time).
CREATE TABLE ohs_sync.pull_jobs (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    provider   text        NOT NULL,
    from_time  timestamptz NOT NULL,
    to_time    timestamptz NOT NULL,
    status     text        NOT NULL DEFAULT 'pending',
    attempts   integer     NOT NULL DEFAULT 0,
    not_before timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pull_jobs_status_check CHECK (status IN ('pending', 'running', 'done', 'parked'))
);
CREATE INDEX pull_jobs_claimable ON ohs_sync.pull_jobs (not_before, id) WHERE status = 'pending';
CREATE INDEX pull_jobs_reaper ON ohs_sync.pull_jobs (claimed_at) WHERE status = 'running';
CREATE INDEX pull_jobs_user_status ON ohs_sync.pull_jobs (tenant_id, user_id, status, created_at DESC);
-- At most one unfinished job per user, provider and window.
CREATE UNIQUE INDEX pull_jobs_idempotent ON ohs_sync.pull_jobs (tenant_id, user_id, provider, from_time, to_time)
    WHERE status IN ('pending', 'running');

CREATE TABLE ohs_sync.pull_coverage (
    tenant_id      uuid        NOT NULL,
    user_id        uuid        NOT NULL,
    provider       text        NOT NULL,
    metric         text        NOT NULL,
    date           date        NOT NULL,
    records_count  integer     NOT NULL DEFAULT 0,
    last_synced_at timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, provider, metric, date)
);

-- One row per pull (stage 'pull') or per processed raw object (stage 'etl').
CREATE TABLE ohs_sync.sync_log (
    id                 bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id          uuid        NOT NULL,
    user_id            uuid        NOT NULL,
    provider           text        NOT NULL,
    stage              text        NOT NULL,
    raw_key            text,
    metric_type        text        NOT NULL DEFAULT 'all',
    status             text        NOT NULL,
    records_written    integer     NOT NULL DEFAULT 0,
    duplicates_skipped integer     NOT NULL DEFAULT 0,
    records_failed     integer     NOT NULL DEFAULT 0,
    duration_ms        bigint      NOT NULL DEFAULT 0,
    error_message      text,
    day_start          timestamptz,
    day_end            timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sync_log_stage_check CHECK (stage IN ('pull', 'etl')),
    -- pull: completed | error; etl: success | partial | failed
    CONSTRAINT sync_log_status_check CHECK (status IN ('completed', 'error', 'success', 'partial', 'failed'))
);
CREATE INDEX sync_log_user_created ON ohs_sync.sync_log (tenant_id, user_id, created_at DESC);
CREATE INDEX sync_log_created ON ohs_sync.sync_log (created_at DESC);
CREATE INDEX sync_log_raw_key ON ohs_sync.sync_log (raw_key) WHERE raw_key IS NOT NULL;

-- Provider-side notification subscriptions (topic = the provider's category, e.g. an appli number).
CREATE TABLE ohs_sync.inbound_subscriptions (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid        NOT NULL,
    user_id          uuid        NOT NULL,
    provider         text        NOT NULL,
    topic            text        NOT NULL,
    callback_url     text        NOT NULL,
    active           boolean     NOT NULL DEFAULT true,
    subscribed_at    timestamptz NOT NULL DEFAULT now(),
    last_received_at timestamptz,
    CONSTRAINT inbound_subscriptions_key UNIQUE (tenant_id, user_id, provider, topic)
);

-- One row per raw file written to the raw store. content_hash dedups identical uploads.
CREATE TABLE ohs_sync.raw_objects (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    user_id      uuid        NOT NULL,
    provider     text        NOT NULL,
    key          text        NOT NULL,
    content_hash text        NOT NULL,
    size_bytes   bigint      NOT NULL,
    batch        text        NOT NULL,
    part         integer     NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT raw_objects_key_key UNIQUE (key),
    CONSTRAINT raw_objects_dedup_key UNIQUE (tenant_id, user_id, content_hash)
);
CREATE INDEX raw_objects_user_created ON ohs_sync.raw_objects (tenant_id, user_id, created_at DESC);
CREATE INDEX raw_objects_created ON ohs_sync.raw_objects (created_at DESC);

-- ETL queue: one job processes one raw object (key).
CREATE TABLE ohs_sync.etl_jobs (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    provider   text        NOT NULL,
    key        text        NOT NULL,
    tier       text        NOT NULL DEFAULT 'standard',
    status     text        NOT NULL DEFAULT 'pending',
    attempts   integer     NOT NULL DEFAULT 0,
    not_before timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT etl_jobs_tier_check CHECK (tier IN ('standard', 'high_freq')),
    CONSTRAINT etl_jobs_status_check CHECK (status IN ('pending', 'running', 'done', 'parked'))
);
CREATE INDEX etl_jobs_claimable ON ohs_sync.etl_jobs (tier, not_before, id) WHERE status = 'pending';
CREATE INDEX etl_jobs_reaper ON ohs_sync.etl_jobs (claimed_at) WHERE status = 'running';
CREATE INDEX etl_jobs_key ON ohs_sync.etl_jobs (key);
CREATE INDEX etl_jobs_user ON ohs_sync.etl_jobs (tenant_id, user_id, status);

-- ===========================================================================
-- ohs_data: hypertables
--
-- Each canonical record carries provider (who Open Health Sync got it from), source_slug
-- (the vendor that produced it), device_id and ingested_at.
-- ===========================================================================

CREATE TABLE ohs_data.quantity_samples (
    id                bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id         uuid        NOT NULL,
    user_id           uuid        NOT NULL,
    provider          text        NOT NULL,
    source_slug       text,
    source_name       text,
    source_bundle_id  text,
    device_id         text,
    device_model      text,
    product_type      text,
    external_id       text,
    quantity_type     text        NOT NULL,
    value             double precision NOT NULL,
    unit              text,
    is_primary_source boolean     DEFAULT false,
    was_user_entered  boolean,
    start_date        timestamptz NOT NULL,
    end_date          timestamptz,
    content_hash      text        NOT NULL,
    metadata          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, start_date),
    CONSTRAINT quantity_samples_dedup_key UNIQUE (tenant_id, user_id, content_hash, start_date)
);
SELECT create_hypertable('ohs_data.quantity_samples', by_range('start_date'));
CREATE INDEX quantity_samples_user_type_date ON ohs_data.quantity_samples (tenant_id, user_id, quantity_type, start_date DESC);
CREATE INDEX quantity_samples_user_date ON ohs_data.quantity_samples (tenant_id, user_id, start_date DESC);

CREATE TABLE ohs_data.category_samples (
    id            bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id     uuid        NOT NULL,
    user_id       uuid        NOT NULL,
    provider      text        NOT NULL,
    source_slug   text,
    source_name   text,
    device_id     text,
    external_id   text,
    category_type text        NOT NULL,
    value_int     integer,
    value_label   text,
    start_date    timestamptz NOT NULL,
    end_date      timestamptz,
    content_hash  text        NOT NULL,
    metadata      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, start_date),
    CONSTRAINT category_samples_dedup_key UNIQUE (tenant_id, user_id, content_hash, start_date)
);
SELECT create_hypertable('ohs_data.category_samples', by_range('start_date'));
CREATE INDEX category_samples_user_type_date ON ohs_data.category_samples (tenant_id, user_id, category_type, start_date DESC);
CREATE INDEX category_samples_user_date ON ohs_data.category_samples (tenant_id, user_id, start_date DESC);

CREATE TABLE ohs_data.sleep_sessions (
    id                  bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id           uuid        NOT NULL,
    user_id             uuid        NOT NULL,
    provider            text        NOT NULL,
    source_slug         text,
    device_id           text,
    external_id         text,
    bedtime_start       timestamptz NOT NULL,
    bedtime_end         timestamptz,
    total_sleep_seconds integer,
    deep_sleep_seconds  integer,
    rem_sleep_seconds   integer,
    light_sleep_seconds integer,
    awake_seconds       integer,
    sleep_score         double precision,
    hrv_avg_ms          double precision,
    resting_hr          double precision,
    respiratory_rate    double precision,
    spo2_avg            double precision,
    content_hash        text        NOT NULL,
    metadata            jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, bedtime_start),
    CONSTRAINT sleep_sessions_dedup_key UNIQUE (tenant_id, user_id, content_hash, bedtime_start)
);
SELECT create_hypertable('ohs_data.sleep_sessions', by_range('bedtime_start'));
CREATE INDEX sleep_sessions_user_bed ON ohs_data.sleep_sessions (tenant_id, user_id, bedtime_start DESC);
CREATE INDEX sleep_sessions_id ON ohs_data.sleep_sessions (id);

CREATE TABLE ohs_data.workouts (
    id                 bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id          uuid        NOT NULL,
    user_id            uuid        NOT NULL,
    provider           text        NOT NULL,
    source_slug        text,
    device_id          text,
    external_id        text,
    activity_type      text        NOT NULL,
    start_date         timestamptz NOT NULL,
    end_date           timestamptz,
    duration_s         integer,
    active_energy_kcal double precision,
    distance_m         double precision,
    avg_heart_rate     double precision,
    max_heart_rate     double precision,
    avg_speed_mps      double precision,
    elevation_gain_m   double precision,
    vo2_max            double precision,
    content_hash       text        NOT NULL,
    metadata           jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, start_date),
    CONSTRAINT workouts_dedup_key UNIQUE (tenant_id, user_id, content_hash, start_date)
);
SELECT create_hypertable('ohs_data.workouts', by_range('start_date'));
CREATE INDEX workouts_user_date ON ohs_data.workouts (tenant_id, user_id, start_date DESC);
CREATE INDEX workouts_id ON ohs_data.workouts (id);

-- Route points keep latitude and longitude as plain double precision (no PostGIS).
CREATE TABLE ohs_data.route_points (
    id           bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id    uuid        NOT NULL,
    user_id      uuid        NOT NULL,
    workout_id   bigint      NOT NULL,
    "timestamp"  timestamptz NOT NULL,
    latitude     double precision NOT NULL,
    longitude    double precision NOT NULL,
    altitude_m   double precision,
    speed_mps    double precision,
    course_deg   double precision,
    content_hash text        NOT NULL,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, "timestamp"),
    CONSTRAINT route_points_dedup_key UNIQUE (tenant_id, user_id, content_hash, "timestamp")
);
SELECT create_hypertable('ohs_data.route_points', by_range('timestamp'));
CREATE INDEX route_points_workout ON ohs_data.route_points (workout_id, "timestamp");
CREATE INDEX route_points_user ON ohs_data.route_points (tenant_id, user_id, "timestamp" DESC);

CREATE TABLE ohs_data.nutrition_logs (
    id             bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id      uuid        NOT NULL,
    user_id        uuid        NOT NULL,
    provider       text        NOT NULL,
    source_slug    text,
    device_id      text,
    external_id    text,
    start_date     timestamptz NOT NULL,
    end_date       timestamptz,
    meal_type      text,
    energy_kcal    double precision,
    protein_g      double precision,
    carbs_g        double precision,
    fat_g          double precision,
    fiber_g        double precision,
    sugar_g        double precision,
    sodium_mg      double precision,
    nutrients_json jsonb,
    content_hash   text        NOT NULL,
    metadata       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, start_date),
    CONSTRAINT nutrition_logs_dedup_key UNIQUE (tenant_id, user_id, content_hash, start_date)
);
SELECT create_hypertable('ohs_data.nutrition_logs', by_range('start_date'));
CREATE INDEX nutrition_logs_user_date ON ohs_data.nutrition_logs (tenant_id, user_id, start_date DESC);

CREATE TABLE ohs_data.ecg_recordings (
    id                     bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id              uuid        NOT NULL,
    user_id                uuid        NOT NULL,
    provider               text        NOT NULL,
    source_slug            text,
    device_id              text,
    start_date             timestamptz NOT NULL,
    end_date               timestamptz,
    classification         integer     NOT NULL,
    symptoms_status        integer     NOT NULL,
    average_heart_rate     double precision,
    sampling_frequency     double precision NOT NULL,
    number_of_measurements integer     NOT NULL,
    voltage_path           text,
    content_hash           text        NOT NULL,
    metadata               jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at            timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, start_date),
    CONSTRAINT ecg_recordings_dedup_key UNIQUE (tenant_id, user_id, content_hash, start_date)
);
SELECT create_hypertable('ohs_data.ecg_recordings', by_range('start_date'));
CREATE INDEX ecg_recordings_user_date ON ohs_data.ecg_recordings (tenant_id, user_id, start_date DESC);
CREATE INDEX ecg_recordings_id ON ohs_data.ecg_recordings (id);

-- One voltage sample of an ECG recording. ts = recording start + time_since_sample_start,
-- the time partition column.
CREATE TABLE ohs_data.ecg_samples (
    id                      bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id               uuid        NOT NULL,
    user_id                 uuid        NOT NULL,
    ecg_id                  bigint      NOT NULL,
    ts                      timestamptz NOT NULL,
    time_since_sample_start double precision NOT NULL,
    voltage_uv              double precision NOT NULL,
    content_hash            text        NOT NULL,
    ingested_at             timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, ts),
    CONSTRAINT ecg_samples_dedup_key UNIQUE (tenant_id, user_id, content_hash, ts)
);
SELECT create_hypertable('ohs_data.ecg_samples', by_range('ts'));
CREATE INDEX ecg_samples_ecg ON ohs_data.ecg_samples (ecg_id, ts);
CREATE INDEX ecg_samples_user ON ohs_data.ecg_samples (tenant_id, user_id, ts DESC);

CREATE TABLE ohs_data.heartbeat_samples (
    tenant_id    uuid        NOT NULL,
    user_id      uuid        NOT NULL,
    provider     text        NOT NULL,
    series_id    text        NOT NULL,
    "timestamp"  timestamptz NOT NULL,
    rr_ms        double precision NOT NULL,
    content_hash text        NOT NULL,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT heartbeat_samples_dedup_key UNIQUE (tenant_id, user_id, content_hash, "timestamp")
);
SELECT create_hypertable('ohs_data.heartbeat_samples', by_range('timestamp'));
CREATE INDEX heartbeat_samples_user ON ohs_data.heartbeat_samples (tenant_id, user_id, "timestamp" DESC);
CREATE INDEX heartbeat_samples_series ON ohs_data.heartbeat_samples (series_id, "timestamp");

-- One row per user, provider and local calendar day (summary_date is a date, not an instant).
CREATE TABLE ohs_data.daily_summaries (
    id              bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id       uuid        NOT NULL,
    user_id         uuid        NOT NULL,
    provider        text        NOT NULL,
    source_slug     text,
    summary_date    date        NOT NULL, -- the user's local calendar day
    readiness_score double precision,
    activity_score  double precision,
    sleep_score     double precision,
    recovery_score  double precision,
    strain_score    double precision,
    step_count      integer,
    active_kcal     double precision,
    total_kcal      double precision,
    distance_m      double precision,
    floors_climbed  integer,
    resting_hr      double precision,
    hrv_avg_ms      double precision,
    content_hash    text        NOT NULL,
    metadata        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, summary_date),
    CONSTRAINT daily_summaries_dedup_key UNIQUE (tenant_id, user_id, content_hash, summary_date)
);
SELECT create_hypertable('ohs_data.daily_summaries', by_range('summary_date'));
CREATE INDEX daily_summaries_user_date ON ohs_data.daily_summaries (tenant_id, user_id, summary_date DESC);
CREATE INDEX daily_summaries_user_provider_date ON ohs_data.daily_summaries (tenant_id, user_id, provider, summary_date);

-- Generic coded observations (anything without a typed table), LOINC/UCUM when known.
CREATE TABLE ohs_data.observations (
    id                bigint      GENERATED BY DEFAULT AS IDENTITY,
    tenant_id         uuid        NOT NULL,
    user_id           uuid        NOT NULL,
    provider          text        NOT NULL,
    source_slug       text,
    source_name       text,
    device_id         text,
    ts                timestamptz NOT NULL,
    end_ts            timestamptz,
    canonical_code    text        NOT NULL,
    value_num         double precision,
    value_code        text,
    value_text        text,
    unit_ucum         text,
    value_canonical   double precision,
    is_primary_source boolean     DEFAULT false,
    type_concept      text,
    source_value      text,
    raw_ref           text,
    content_hash      text        NOT NULL,
    metadata          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ingested_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, ts),
    CONSTRAINT observations_dedup_key UNIQUE (tenant_id, user_id, content_hash, ts)
);
SELECT create_hypertable('ohs_data.observations', by_range('ts'));
CREATE INDEX observations_user_code ON ohs_data.observations (tenant_id, user_id, canonical_code, ts DESC);
CREATE INDEX observations_user_ts ON ohs_data.observations (tenant_id, user_id, ts DESC);

-- ===========================================================================
-- ohs_data: plain tables
-- ===========================================================================

-- Child of sleep_sessions (session_id = sleep_sessions.id). No foreign key: the
-- parent is a hypertable whose unique keys all include bedtime_start.
CREATE TABLE ohs_data.sleep_phases (
    id           bigint      GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    user_id      uuid        NOT NULL,
    session_id   bigint      NOT NULL,
    phase        text        NOT NULL,
    start_date   timestamptz NOT NULL,
    end_date     timestamptz,
    duration_s   integer     NOT NULL,
    content_hash text        NOT NULL,
    ingested_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sleep_phases_dedup_key UNIQUE (tenant_id, user_id, content_hash)
);
CREATE INDEX sleep_phases_session ON ohs_data.sleep_phases (session_id, start_date);
CREATE INDEX sleep_phases_user ON ohs_data.sleep_phases (tenant_id, user_id, start_date);

-- One summary per traced workout, derived from route_points and rebuilt at will.
CREATE TABLE ohs_data.workout_tracks (
    tenant_id   uuid        NOT NULL,
    user_id     uuid        NOT NULL,
    workout_id  bigint      NOT NULL,
    length_m    double precision NOT NULL,
    point_count integer     NOT NULL,
    started_at  timestamptz NOT NULL,
    ended_at    timestamptz NOT NULL,
    built_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, workout_id)
);
CREATE INDEX workout_tracks_user_started ON ohs_data.workout_tracks (tenant_id, user_id, started_at DESC);

-- Reference: canonical quantity codes and their standard terminologies. Not per tenant.
CREATE TABLE ohs_data.quantity_codes (
    canonical_code text        PRIMARY KEY,
    loinc_code     text,
    snomed_code    text,
    ucum_unit      text,
    storage_unit   text,
    value_type     text        NOT NULL DEFAULT 'num',
    hot_table      text,
    promoted       boolean     NOT NULL DEFAULT false,
    updated_at     timestamptz NOT NULL DEFAULT now()
);

-- Reference: patterns that resolve a source/device string to a device key. Not per tenant.
CREATE TABLE ohs_data.device_aliases (
    match_kind text    NOT NULL,
    pattern    text    NOT NULL,
    provider   text    NOT NULL DEFAULT '',
    device_key text    NOT NULL,
    grain      text    NOT NULL,
    priority   integer NOT NULL DEFAULT 100,
    PRIMARY KEY (match_kind, pattern, provider)
);

-- Records (line_number set) or whole raw objects (line_number NULL) the ETL could not write.
CREATE TABLE ohs_data.failed_records (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     uuid        NOT NULL,
    user_id       uuid        NOT NULL,
    provider      text        NOT NULL,
    raw_key       text        NOT NULL,
    line_number   integer,
    raw_payload   jsonb,
    reason_code   text,
    error_message text,
    status        text        NOT NULL DEFAULT 'pending',
    attempts      integer     NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    recovered_at  timestamptz,
    CONSTRAINT failed_records_status_check CHECK (status IN ('pending', 'recovered', 'permanent')),
    CONSTRAINT failed_records_reason_code_check
        CHECK (reason_code IN ('parse', 'mapping', 'validation', 'unit', 'unknown', 'job'))
);
CREATE INDEX failed_records_pending ON ohs_data.failed_records (raw_key) WHERE status = 'pending';
CREATE INDEX failed_records_user ON ohs_data.failed_records (tenant_id, user_id, provider, status);
CREATE INDEX failed_records_created ON ohs_data.failed_records (created_at DESC);
