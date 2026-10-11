-- SPDX-License-Identifier: AGPL-3.0-only
-- A push subscription is a durable row: written pending when a user connects,
-- subscribed at the provider by the collect role (retried with backoff), and
-- removed (unsubscribed at the provider) on disconnect or user deletion.
ALTER TABLE ohs_sync.inbound_subscriptions
    ALTER COLUMN subscribed_at DROP NOT NULL,
    ALTER COLUMN subscribed_at DROP DEFAULT,
    ADD COLUMN status text NOT NULL DEFAULT 'pending'
        CONSTRAINT inbound_subscriptions_status_check CHECK (status IN ('pending', 'subscribed', 'removed')),
    ADD COLUMN attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN last_error text,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

-- Rows written before this migration were live at the provider when active:
-- they stay subscribed (a pending row would subscribe them a second time).
-- An inactive row was already unsubscribed.
UPDATE ohs_sync.inbound_subscriptions
    SET status = CASE WHEN active THEN 'subscribed' ELSE 'removed' END;

ALTER TABLE ohs_sync.inbound_subscriptions DROP COLUMN active;

-- The collect role looks for due pending rows.
CREATE INDEX inbound_subscriptions_due ON ohs_sync.inbound_subscriptions (next_attempt_at)
    WHERE status = 'pending';
