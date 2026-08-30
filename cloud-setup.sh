#!/usr/bin/env bash
# Setup script for a Claude Code cloud environment.
#
# Paste this into the Setup script field at claude.ai/code for this repo's
# environment. It runs ONCE as root on Ubuntu 24.04 x86_64; the filesystem is
# snapshotted afterwards, so later sessions reuse everything installed here.
#
# NOTE ON .devcontainer/: cloud sessions do NOT read it. It remains in this
# repo for plain-Docker and local use only. This script is the cloud path.
#
# Budget: the setup field allows roughly five minutes. Steps are ordered so the
# cheapest, most load-bearing tools land first; if the budget is exceeded, the
# tail of this script is what to move into a session-time step.
set -euo pipefail

log() { printf '\n=== %s ===\n' "$*"; }

# --- What the base image already provides -------------------------------
# Go, Rust (rustc/cargo), Docker, OpenJDK 21, git, gh, jq, python3.
# What it does NOT provide, and this project needs:
#   JDK 17   -- the findings were produced on 17; Kotlin targets bytecode 17
#   kotlinc  -- absent entirely
#   Verus    -- absent; Rust deductive verifier
#   CBMC     -- absent; provides jbmc, and F014 is a defect in 6.11.0 exactly
#   Gobra    -- a jar, distributed only inside a container image

log "JDK 17 (findings were produced on 17, base image ships 21)"
apt-get update -qq
apt-get install -y -qq --no-install-recommends openjdk-17-jdk unzip >/dev/null
# Do not switch the default java; leave 21 as-is and address 17 explicitly.
export JAVA17_HOME=/usr/lib/jvm/java-17-openjdk-amd64
echo "JAVA17_HOME=$JAVA17_HOME" >> /etc/environment

log "CBMC 6.11.0 (jbmc) -- pinned; F014 IS a defect in this build"
curl -fsSL -o /tmp/cbmc.deb \
  "https://github.com/diffblue/cbmc/releases/download/cbmc-6.11.0/ubuntu-24.04-cbmc-6.11.0-Linux.deb"
apt-get install -y -qq /tmp/cbmc.deb >/dev/null && rm -f /tmp/cbmc.deb

log "Verus 0.2026.04.24.f8e1704 + its bundled Z3"
mkdir -p /opt/verus && cd /opt/verus
V=0.2026.04.24.f8e1704
curl -fsSL -o v.zip \
  "https://github.com/verus-lang/verus/releases/download/release%2F${V}/verus-${V}-x86-linux.zip"
unzip -q v.zip && rm v.zip
# The directory name inside the archive is not documented; resolve it rather
# than assuming, because assuming it is exactly how the local Dockerfile broke.
VERUS_BIN="$(find /opt/verus -maxdepth 3 -type f -name verus -perm -u+x | head -1)"
if [ -z "$VERUS_BIN" ]; then echo "FATAL: verus binary not found after unzip"; exit 1; fi
VERUS_DIR="$(dirname "$VERUS_BIN")"
echo "VERUS_PATH=$VERUS_BIN"   >> /etc/environment
echo "PATH=$VERUS_DIR:\$PATH"  >> /etc/environment
ln -sf "$VERUS_BIN" /usr/local/bin/verus
# Z3 ships beside Verus and Gobra needs one on PATH.
[ -x "$VERUS_DIR/z3" ] && ln -sf "$VERUS_DIR/z3" /usr/local/bin/z3

log "Kotlin 2.4.10"
curl -fsSL -o /tmp/kotlin.zip \
  "https://github.com/JetBrains/kotlin/releases/download/v2.4.10/kotlin-compiler-2.4.10.zip"
unzip -q /tmp/kotlin.zip -d /opt && rm /tmp/kotlin.zip
ln -sf /opt/kotlinc/bin/kotlinc /usr/local/bin/kotlinc

log "Gobra jar, lifted out of its image (no daemon needed at run time)"
# viperproject/gobra publishes no release assets, so the image is the only
# distribution channel. Docker images pulled during setup are snapshotted, but
# the jar is what matters -- extract it so the rung needs only a JVM and Z3.
GOBRA_IMG="ghcr.io/viperproject/gobra@sha256:2ef080ccd284945829501996e6d63ed2f1c94b7cf6a30d2b934272fb8a6df2c6"
mkdir -p /opt/gobra
if docker pull -q "$GOBRA_IMG" >/dev/null 2>&1; then
  CID="$(docker create "$GOBRA_IMG")"
  docker cp "$CID:/gobra/gobra.jar" /opt/gobra/gobra.jar
  docker rm -f "$CID" >/dev/null
  echo "GOBRA_JAR=/opt/gobra/gobra.jar" >> /etc/environment
else
  echo "WARN: gobra image pull failed -- the Go corner will be limited to R0-R3."
  echo "      Everything else is unaffected. Re-run this block in a session to fix."
fi

log "Toolchain as installed"
{
  printf '  %-10s %s\n' go      "$(go version 2>&1 || echo ABSENT)"
  printf '  %-10s %s\n' rustc   "$(rustc --version 2>&1 || echo ABSENT)"
  printf '  %-10s %s\n' verus   "$(verus --version 2>&1 | grep -i version | head -1 || echo ABSENT)"
  printf '  %-10s %s\n' z3      "$(z3 --version 2>&1 || echo ABSENT)"
  printf '  %-10s %s\n' jbmc    "$(jbmc --version 2>&1 || echo ABSENT)"
  printf '  %-10s %s\n' kotlinc "$(kotlinc -version 2>&1 | head -1 || echo ABSENT)"
  printf '  %-10s %s\n' jdk17   "$([ -x "$JAVA17_HOME/bin/java" ] && "$JAVA17_HOME/bin/java" -version 2>&1 | head -1 || echo ABSENT)"
  printf '  %-10s %s\n' gobra   "$([ -f /opt/gobra/gobra.jar ] && echo "$(stat -c%s /opt/gobra/gobra.jar) bytes" || echo ABSENT)"
} 2>&1

log "Setup complete"
echo "Verify inside a session with:  go run ./tools/cmd/matrixctl doctor"
