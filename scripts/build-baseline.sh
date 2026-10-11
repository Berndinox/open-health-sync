#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Documents how internal/db/migrations/0001_baseline.up.sql was made. It is not
# part of the build and never runs in CI.
#
# Open Health Sync started as a port of a private source backend. That backend split its
# schema across several services, each with its own golang-migrate directory.
# This script replays those directories, in order, on a scratch TimescaleDB
# container, then dumps the final schema to a file OUTSIDE the repository
# (the dump still carries the old names). The baseline was then curated by
# hand from that dump: tables renamed, moved to ohs_core, ohs_sync and
# ohs_data, tenant_id added, dropped features removed.
#
# Usage:
#   OHS_BASELINE_SRC=/path/to/private/source/backend \
#     scripts/build-baseline.sh <migrations dir> [<migrations dir>...]
#
# Each directory is relative to OHS_BASELINE_SRC and is applied into its own
# scratch schema (src1, src2, ...), as the source monolith did with one schema
# per service. Up files that need PostGIS are skipped (Open Health Sync has no geo tables).
#
# Env:
#   OHS_BASELINE_SRC    root of the private source backend checkout (required)
#   OHS_BASELINE_OUT    dump path (default /tmp/ohs-baseline-raw.sql)
#   OHS_BASELINE_IMAGE  default timescale/timescaledb:latest-pg17
set -euo pipefail

: "${OHS_BASELINE_SRC:?set OHS_BASELINE_SRC to the private source backend checkout}"
out="${OHS_BASELINE_OUT:-/tmp/ohs-baseline-raw.sql}"
image="${OHS_BASELINE_IMAGE:-timescale/timescaledb:latest-pg17}"
[ "$#" -ge 1 ] || { echo "usage: $0 <migrations dir>..." >&2; exit 2; }

# Resolve the dump path to an absolute, symlink-free one before checking it is
# outside the repository (relative paths and ../ tricks included).
out_dir="$(dirname "$out")"
[ -d "$out_dir" ] || { echo "dump directory $out_dir does not exist" >&2; exit 2; }
out="$(cd "$out_dir" && pwd -P)/$(basename "$out")"
repo="$(git -C "$(dirname "$0")" rev-parse --show-toplevel 2>/dev/null || true)"
if [ -n "$repo" ]; then
  repo="$(cd "$repo" && pwd -P)"
  case "$out" in
    "$repo"/*) echo "refusing to write the raw dump inside the repository ($out)" >&2; exit 2 ;;
  esac
fi

name="ohs-baseline-$$"
docker run -d --rm --name "$name" -e POSTGRES_PASSWORD=scratch "$image" >/dev/null
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT

until docker exec "$name" pg_isready -U postgres -q 2>/dev/null; do sleep 1; done
# The entrypoint restarts the server once after init; wait for the final one.
sleep 3
until docker exec "$name" psql -U postgres -qtAc 'select 1' >/dev/null 2>&1; do sleep 1; done

psql_in() { docker exec -i "$name" psql -U postgres -v ON_ERROR_STOP=1 -q "$@"; }

psql_in -c 'CREATE EXTENSION IF NOT EXISTS timescaledb'

i=0
for dir in "$@"; do
  i=$((i + 1))
  schema="src$i"
  psql_in -c "CREATE SCHEMA $schema"
  for f in "$OHS_BASELINE_SRC/$dir"/*.up.sql; do
    if grep -qi 'postgis' "$f"; then
      echo "skip (PostGIS) $(basename "$f")"
      continue
    fi
    echo "apply $schema $(basename "$f")"
    { echo "SET search_path = $schema, public;"; cat "$f"; } | psql_in
  done
done

docker exec "$name" pg_dump -U postgres --schema-only --no-owner --no-privileges postgres > "$out"
# pg_dump does not print create_hypertable calls: append them as comments.
docker exec "$name" psql -U postgres -qtA -c "
  SELECT '-- hypertable ' || hypertable_schema || '.' || hypertable_name || ' by ' || column_name
  FROM timescaledb_information.dimensions ORDER BY 1" >> "$out"
echo "dump written to $out"
