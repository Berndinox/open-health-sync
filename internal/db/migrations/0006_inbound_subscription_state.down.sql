-- SPDX-License-Identifier: AGPL-3.0-only
DROP INDEX ohs_sync.inbound_subscriptions_due;
DELETE FROM ohs_sync.inbound_subscriptions WHERE subscribed_at IS NULL;
ALTER TABLE ohs_sync.inbound_subscriptions
    DROP COLUMN updated_at,
    DROP COLUMN last_error,
    DROP COLUMN next_attempt_at,
    DROP COLUMN attempts,
    ADD COLUMN active boolean NOT NULL DEFAULT true,
    ALTER COLUMN subscribed_at SET DEFAULT now(),
    ALTER COLUMN subscribed_at SET NOT NULL;
UPDATE ohs_sync.inbound_subscriptions SET active = (status = 'subscribed');
ALTER TABLE ohs_sync.inbound_subscriptions DROP COLUMN status;
