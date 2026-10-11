-- SPDX-License-Identifier: AGPL-3.0-only
-- One raw object holds one data type. The key alone does not name it and the
-- uploaded lines are stored verbatim, so the type lives on the registry row.
ALTER TABLE ohs_sync.raw_objects ADD COLUMN data_type text NOT NULL DEFAULT '';
