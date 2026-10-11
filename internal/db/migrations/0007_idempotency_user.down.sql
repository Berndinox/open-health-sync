-- SPDX-License-Identifier: AGPL-3.0-only
DROP INDEX ohs_core.idempotency_keys_user;
ALTER TABLE ohs_core.idempotency_keys DROP CONSTRAINT idempotency_keys_user_fk,
    DROP COLUMN user_id;
