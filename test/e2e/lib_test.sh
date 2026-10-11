#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests of the lib.sh helpers that decide pass or fail. No server, no
# container: it runs first in `make e2e`.
set -euo pipefail

E2E_PROJECT=ohs-libtest
OHS_E2E_PORT=1 # nothing listens there
# shellcheck source=test/e2e/lib.sh
source "$(dirname "$0")/lib.sh"
trap 'rm -rf "$E2E_WORK"' EXIT

T_FAILS=0
expect() { # DESCRIPTION WANT_STATUS(pass|fail) COMMAND...
  local what=$1 want=$2 got=pass
  shift 2
  if ! "$@" >/dev/null 2>&1; then got=fail; fi
  if [ "$got" = "$want" ]; then
    echo "ok   $what"
  else
    echo "FAIL $what: wanted $want, got $got"
    T_FAILS=$((T_FAILS + 1))
  fi
}

# check_fails LABEL GOT WANT: true when check counted exactly one failure.
check_counts() {
  local before=$E2E_FAILS
  check "$@" >/dev/null
  [ "$E2E_FAILS" -eq $((before + 1)) ]
}
check_passes() {
  local before=$E2E_FAILS
  check "$@" >/dev/null
  [ "$E2E_FAILS" -eq "$before" ]
}

expect "check passes on equal integers" pass check_passes n 288 288
expect "check passes on equal decimals" pass check_passes n 19440.000 19440
expect "check passes on negatives" pass check_passes n -1 -1.0
expect "check fails on different numbers" pass check_counts n 287 288
expect "check fails on an empty value" pass check_counts n "" 288
expect "check fails on an empty want" pass check_counts n 0 ""
expect "check fails on two empty values" pass check_counts n "" ""
expect "check fails on non-numeric" pass check_counts n abc abc
expect "check fails on a number with junk" pass check_counts n "12 rows" 12
expect "check fails on NaN" pass check_counts n nan nan
expect "check fails on exponent form" pass check_counts n 1e3 1000

decides() { [ "$(kill_decision "$1" "$2" "$3")" = "$4" ]; }
expect "kill waits while no row has landed" pass decides 0 100 6 wait
expect "kill once rows are partly landed" pass decides 40 100 2 partial
expect "kill is late once no job is left" pass decides 100 100 0 late
expect "kill waits on an unreadable progress" pass decides "" 100 2 wait
expect "kill waits when every row landed but jobs remain" pass decides 100 100 1 wait

expect "api fails when no server answers" fail api GET /healthz none
expect "e2e_count fails when no server answers" fail e2e_count k u /sleep

expect "build flags of deploy/Dockerfile match the host fallback" pass \
  e2e_build_flags_match "$E2E_ROOT/deploy/Dockerfile"
sed 's/-trimpath/-trimpath -race/' "$E2E_ROOT/deploy/Dockerfile" >"$E2E_WORK/Dockerfile.drift"
expect "build flags drift is refused" fail e2e_build_flags_match "$E2E_WORK/Dockerfile.drift"

E2E_ROOT_SAVED=$E2E_ROOT
mkdir -p "$E2E_WORK/root"
grep -v '^OHS_PUBLIC_URL=' "$E2E_ROOT/.env.example" >"$E2E_WORK/root/.env.example"
E2E_ROOT="$E2E_WORK/root"
expect "e2e_env refuses an .env.example without OHS_PUBLIC_URL" fail e2e_env
E2E_ROOT=$E2E_ROOT_SAVED
expect "e2e_env writes the env file" pass e2e_env
expect "the env file has a 32-byte key" pass \
  bash -c "grep -q '^OHS_ENCRYPTION_KEY=.\{44\}$' '$OHS_E2E_ENV_FILE'"

if docker info >/dev/null 2>&1; then
  expect "psql_q fails when the project does not run" fail psql_q 'SELECT 1'
fi

if [ "$T_FAILS" -ne 0 ]; then
  echo "lib_test: $T_FAILS failure(s)"
  exit 1
fi
echo "lib_test: all passed"
