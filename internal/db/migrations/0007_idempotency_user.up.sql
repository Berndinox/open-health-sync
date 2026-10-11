-- SPDX-License-Identifier: AGPL-3.0-only
-- A stored response that names an end user (POST /v1/users) records that
-- user, so deleting the user also deletes the stored response. The foreign
-- key cascades as a safety net; the deletion paths delete the rows
-- explicitly. NULL for responses that name no user.
ALTER TABLE ohs_core.idempotency_keys ADD COLUMN user_id uuid,
    ADD CONSTRAINT idempotency_keys_user_fk FOREIGN KEY (tenant_id, user_id)
        REFERENCES ohs_core.end_users (tenant_id, id) ON DELETE CASCADE;
CREATE INDEX idempotency_keys_user ON ohs_core.idempotency_keys (tenant_id, user_id)
    WHERE user_id IS NOT NULL;
