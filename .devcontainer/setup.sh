#!/usr/bin/env bash
# Post-create: warm caches and pull the one pinned image, then report honestly
# what this environment can and cannot verify.
set -uo pipefail

echo "== warming build caches =="
go build ./... 2>&1 | tail -3
( cd impls/go && go build ./... 2>&1 | tail -3 )
( cd impls/rust && cargo build --release -p server 2>&1 | tail -3 )

echo
echo "== gobra (jar, no daemon) =="
if [ -f "${GOBRA_JAR:-/opt/gobra/gobra.jar}" ]; then
  java -Xss128m -jar "${GOBRA_JAR:-/opt/gobra/gobra.jar}" --help >/dev/null 2>&1 \
    && echo "  gobra jar runs; z3 = $(command -v z3 || echo MISSING)" \
    || echo "  gobra jar present but did not start"
else
  echo "  GOBRA_JAR missing -- the Go corner drops to R0-R3"
fi

echo
echo "== what this environment can verify =="
bash -c 'go run ./tools/cmd/matrixctl doctor' || true
