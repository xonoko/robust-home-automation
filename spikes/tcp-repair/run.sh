#!/usr/bin/env bash
# Runs the spike: the device stays connected while its connection is migrated
# node-a -> node-b -> node-a several times. Exits non-zero on any loss,
# reordering or disconnect.
set -euo pipefail
cd "$(dirname "$0")"

ROUNDS=${ROUNDS:-3}

cleanup() { docker compose down --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker compose up -d --build
sleep 3

from=node-a; to=172.30.0.12
for i in $(seq 1 "$ROUNDS"); do
  echo "==> migration $i: $from -> $to"
  docker compose exec -T "$from" spike migrate "http://$to:9000"
  echo
  sleep 2
  if [ "$from" = node-a ]; then from=node-b; to=172.30.0.11; else from=node-a; to=172.30.0.12; fi
done

docker compose kill -s SIGTERM device >/dev/null
sleep 2
echo "==> device output"
docker compose logs --no-log-prefix device
code=$(docker inspect -f '{{.State.ExitCode}}' "$(docker compose ps -aq device)")
echo "device exit code: $code"
exit "$code"
