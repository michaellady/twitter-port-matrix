#!/usr/bin/env bash
# Post-create: warm caches and pull the one pinned image, then report honestly
# what this environment can and cannot verify.
set -uo pipefail

echo "== warming build caches =="
go build ./... 2>&1 | tail -3
( cd impls/go && go build ./... 2>&1 | tail -3 )
( cd impls/rust && cargo build --release -p server 2>&1 | tail -3 )

echo
echo "== pulling the pinned Gobra image (R4, Go corner) =="
GOBRA=$(jq -r '.tools.gobra.image + "@" + .tools.gobra.digest' docker/pins.json 2>/dev/null)
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  docker pull "$GOBRA" || echo "  gobra pull FAILED -- the Go corner drops to R0-R2"
else
  echo "  no docker daemon -- Gobra (Go R4) unavailable; every other rung is fine"
fi

echo
echo "== what this environment can verify =="
bash -c 'go run ./tools/cmd/matrixctl doctor' || true
