#!/usr/bin/env bash
# Runs at the start of every Claude Code session, local and cloud.
#
# Deliberately does NOT install anything. Toolchain installation belongs in the
# cloud environment's Setup script (cloud-setup.sh), which runs once and is
# snapshotted; repeating it per session would waste minutes and, worse, would
# make "it worked last time" depend on a network call.
#
# This only reports what is actually present, so a session that cannot reach a
# rung says so at the top rather than discovering it three commands in.
set -uo pipefail

where="local"
[ -n "${CLAUDE_CODE_REMOTE:-}" ] && where="cloud"

have() { command -v "$1" >/dev/null 2>&1 && echo yes || echo NO; }

echo "twitter_port_matrix — $where session"
printf '  go %s · rust %s · java %s · kotlinc %s · verus %s · z3 %s · jbmc %s · gobra-jar %s\n' \
  "$(have go)" "$(have rustc)" "$(have java)" "$(have kotlinc)" \
  "$(have verus)" "$(have z3)" "$(have jbmc)" \
  "$([ -f "${GOBRA_JAR:-/opt/gobra/gobra.jar}" ] && echo yes || echo NO)"

# Name the consequence, not just the absence — a missing tool should say which
# rung it costs, because that is the question the reader actually has.
command -v verus  >/dev/null 2>&1 || echo "  ! verus absent  -> Rust R4 unavailable"
command -v jbmc   >/dev/null 2>&1 || echo "  ! jbmc absent   -> Java/Kotlin bounded rung unavailable"
# The Gobra jar is the one absence with a known, specific, non-obvious cause,
# so it gets the cause rather than only the consequence. See CLOUD.md.
if [ ! -f "${GOBRA_JAR:-/opt/gobra/gobra.jar}" ]; then
  echo "  ! gobra absent  -> Go R4 unavailable, and R5 with it (31 of 42 discharged clauses are Gobra-backed)"
  [ "$where" = "cloud" ] && echo "                     cause: ghcr.io blobs come from pkg-containers.githubusercontent.com, which Trusted blocks (403)"
fi
command -v kotlinc >/dev/null 2>&1 || echo "  ! kotlinc absent -> Kotlin corner cannot build"

echo "  full check: go run ./tools/cmd/matrixctl doctor"
exit 0
