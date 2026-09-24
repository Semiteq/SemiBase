#!/usr/bin/env bash
# Waits for the PostgreSQL inside a container to answer. A timeout fails here with a named
# error rather than falling through, which would hand the next step a diagnosis of its own.
set -euo pipefail

container=${1:-}
[ -n "$container" ] || { echo "usage: wait-for-postgres.sh <container>" >&2; exit 2; }

for _ in $(seq 60); do
  if docker exec "$container" pg_isready -h 127.0.0.1 -q 2>/dev/null; then
    exit 0
  fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$container")" != true ]; then
    echo "::error::$container stopped before its PostgreSQL answered"
    docker logs "$container"
    exit 1
  fi
  sleep 2
done

echo "::error::PostgreSQL in $container did not answer within 120 s"
docker logs "$container"
exit 1
