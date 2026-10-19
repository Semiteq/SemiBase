#!/usr/bin/env bash
# Fails when a password reaches the server log of a postgres container. The log is read from
# docker logs, so the server must not run the logging collector. The semiplot login's probe must
# be in it: semibase runs that statement on a connection without the log guard, so a log lacking
# it proves nothing about the password statements.
set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: check-server-log.sh <container> <password>..." >&2
  exit 2
fi
container=$1
shift

collector=$(docker exec "$container" psql -U postgres -qtAc 'SHOW logging_collector')
if [ "$collector" != off ]; then
  echo "::error::logging_collector is '$collector' in $container, so docker logs is not the whole server log"
  exit 1
fi

log=$(mktemp)
trap 'rm -f "$log"' EXIT
# a file, not a pipe: grep -q closing early fails docker logs under pipefail
docker logs "$container" > "$log" 2>&1

if ! grep -qF -- 'FROM public.trends LIMIT 1) probe' "$log"; then
  echo "::error::the server log of $container lacks the semiplot login's probe, so statements are not logged"
  exit 1
fi
for password in "$@"; do
  if grep -qF -- "$password" "$log"; then
    echo "::error::a role password reached the server log of $container"
    exit 1
  fi
done
