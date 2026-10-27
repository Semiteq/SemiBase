#!/usr/bin/env bash
# Fails when a password reaches the server log of a postgres container. The probe grep matches
# plotProbe in internal/provision/check.go.
set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: check-server-log.sh <container> <password>..." >&2
  exit 2
fi
container=$1
shift

collector=$(docker exec "$container" psql -U postgres -qtAc 'SHOW logging_collector')
if [ "$collector" != off ]; then
  echo "::error::logging_collector is '$collector' in $container," \
    "so docker logs is not the whole server log"
  exit 1
fi

log=$(mktemp)
trap 'rm -f "$log"' EXIT
# a file, not a pipe: grep -q closing early fails docker logs under pipefail
docker logs "$container" > "$log" 2>&1

if ! grep -qF -- 'FROM public.trends LIMIT 1) probe' "$log"; then
  echo "::error::the server log of $container lacks the semiplot login's probe:" \
    "either the server does not log statements, or semibase skipped the login check" \
    "(a socket host)"
  exit 1
fi
for password in "$@"; do
  if grep -qF -- "$password" "$log"; then
    echo "::error::a role password reached the server log of $container"
    exit 1
  fi
done
