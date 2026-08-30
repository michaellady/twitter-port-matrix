#!/usr/bin/env bash
# Setup script for a Claude Code cloud environment.
#
# Paste into the Setup script field at claude.ai/code. Runs ONCE as root on
# Ubuntu 24.04 x86_64; the filesystem is snapshotted afterwards.
#
# NOTE: .devcontainer/ is NOT read by cloud sessions. It exists for the plain
# Docker path only. This script is the cloud path.
#
# DESIGN NOTE — why this does not use `set -e` globally.
# The first version did, and died at its first command: the base image ships
# third-party PPAs (deadsnakes, ondrej/php) that the Trusted network allowlist
# blocks with 403, so `apt-get update` exits nonzero over repositories this
# project never uses. Aborting the whole install for that is wrong.
#
# Instead: every step is individually tolerant, and every tool is verified by
# RUNNING IT rather than by trusting an installer's exit code. The script ends
# with an inventory and a non-zero exit only if something load-bearing is
# genuinely missing.
set -uo pipefail

log()  { printf '\n=== %s ===\n' "$*"; }
warn() { printf '  !! %s\n' "$*"; }

# --- apt, made usable ---------------------------------------------------
log "apt sources"
# Disable third-party PPAs the allowlist blocks. Only the Ubuntu archive is
# needed here (openjdk-17, unzip, and CBMC's dependencies).
disabled=0
for f in /etc/apt/sources.list.d/*.list /etc/apt/sources.list.d/*.sources; do
  [ -e "$f" ] || continue
  if grep -qiE 'ppa\.launchpad|launchpadcontent' "$f" 2>/dev/null; then
    mv "$f" "$f.disabled" && disabled=$((disabled+1))
  fi
done
echo "  disabled $disabled third-party PPA source file(s) blocked by the allowlist"
apt-get update -qq 2>&1 | grep -vE '^(Get|Hit|Ign)' | head -5 || true

# --- JDK 17 (optional; base ships 21) -----------------------------------
log "JDK 17"
# The findings were produced on 17. Nothing in them depends on the JVM version
# -- F014 is a CBMC defect and TLC's state count is deterministic -- so 21 is an
# acceptable fallback. Recorded either way rather than assumed.
JAVA17_HOME=""
if apt-get install -y -qq --no-install-recommends openjdk-17-jdk >/dev/null 2>&1; then
  JAVA17_HOME=/usr/lib/jvm/java-17-openjdk-amd64
fi
if [ -n "$JAVA17_HOME" ] && [ -x "$JAVA17_HOME/bin/java" ]; then
  echo "JAVA17_HOME=$JAVA17_HOME" >> /etc/environment
  echo "  installed: $("$JAVA17_HOME/bin/java" -version 2>&1 | head -1)"
else
  warn "JDK 17 unavailable; falling back to the base JDK"
  warn "$(java -version 2>&1 | head -1) -- acceptable, but note it in any result"
fi

apt-get install -y -qq --no-install-recommends unzip >/dev/null 2>&1 || true
command -v unzip >/dev/null || warn "unzip missing -- Verus and Kotlin steps will fail"

# --- CBMC 6.11.0 (jbmc) -------------------------------------------------
log "CBMC 6.11.0 -- pinned; F014 IS a defect in this exact build"
if curl -fsSL --retry 2 -o /tmp/cbmc.deb \
   "https://github.com/diffblue/cbmc/releases/download/cbmc-6.11.0/ubuntu-24.04-cbmc-6.11.0-Linux.deb"; then
  apt-get install -y -qq /tmp/cbmc.deb >/dev/null 2>&1 || dpkg -i /tmp/cbmc.deb >/dev/null 2>&1 || true
  rm -f /tmp/cbmc.deb
fi
command -v jbmc >/dev/null && echo "  $(jbmc --version 2>&1)" || warn "jbmc absent -> Java/Kotlin bounded rung unavailable"

# --- Verus + bundled Z3 -------------------------------------------------
log "Verus 0.2026.04.24.f8e1704"
V=0.2026.04.24.f8e1704
mkdir -p /opt/verus && cd /opt/verus
if curl -fsSL --retry 2 -o v.zip \
   "https://github.com/verus-lang/verus/releases/download/release%2F${V}/verus-${V}-x86-linux.zip"; then
  unzip -q -o v.zip && rm -f v.zip
fi
# Resolve the binary rather than assuming the archive's directory name --
# assuming it is exactly how the local Dockerfile broke.
VERUS_BIN="$(find /opt/verus -maxdepth 3 -type f -name verus -perm -u+x 2>/dev/null | head -1)"
if [ -n "$VERUS_BIN" ]; then
  ln -sf "$VERUS_BIN" /usr/local/bin/verus
  echo "VERUS_PATH=$VERUS_BIN" >> /etc/environment
  Z3="$(dirname "$VERUS_BIN")/z3"
  [ -x "$Z3" ] && ln -sf "$Z3" /usr/local/bin/z3
  echo "  $(verus --version 2>&1 | grep -i version | head -1)"
  echo "  z3: $(z3 --version 2>&1 || echo ABSENT)"
else
  warn "verus binary not found -> Rust R4 unavailable"
fi

# --- Kotlin -------------------------------------------------------------
log "Kotlin 2.4.10"
if curl -fsSL --retry 2 -o /tmp/kotlin.zip \
   "https://github.com/JetBrains/kotlin/releases/download/v2.4.10/kotlin-compiler-2.4.10.zip"; then
  unzip -q -o /tmp/kotlin.zip -d /opt && rm -f /tmp/kotlin.zip
  [ -x /opt/kotlinc/bin/kotlinc ] && ln -sf /opt/kotlinc/bin/kotlinc /usr/local/bin/kotlinc
fi
command -v kotlinc >/dev/null && echo "  $(kotlinc -version 2>&1 | head -1)" || warn "kotlinc absent -> Kotlin corner cannot build"

# --- Gobra jar, lifted from its image -----------------------------------
log "Gobra jar (needs only a JVM and Z3 at run time)"
GOBRA_IMG="ghcr.io/viperproject/gobra@sha256:2ef080ccd284945829501996e6d63ed2f1c94b7cf6a30d2b934272fb8a6df2c6"
mkdir -p /opt/gobra
if docker pull -q "$GOBRA_IMG" >/dev/null 2>&1; then
  CID="$(docker create "$GOBRA_IMG" 2>/dev/null)"
  if [ -n "$CID" ]; then
    docker cp "$CID:/gobra/gobra.jar" /opt/gobra/gobra.jar >/dev/null 2>&1
    docker rm -f "$CID" >/dev/null 2>&1
  fi
fi
if [ -f /opt/gobra/gobra.jar ]; then
  echo "GOBRA_JAR=/opt/gobra/gobra.jar" >> /etc/environment
  echo "  $(stat -c%s /opt/gobra/gobra.jar) bytes"
else
  warn "gobra jar absent -> Go R4 unavailable; every other rung is unaffected"
fi

# --- Inventory, by running each tool ------------------------------------
log "Inventory"
missing=0
check() { # name, command, rung-cost, fatal(0/1)
  if out="$(eval "$2" 2>&1 | head -1)" && [ -n "$out" ]; then
    printf '  ok    %-9s %s\n' "$1" "$out"
  else
    printf '  ABSENT %-8s -> %s\n' "$1" "$3"
    [ "$4" = 1 ] && missing=$((missing+1))
  fi
}
check go      "go version"                    "nothing runs"                  1
check rustc   "rustc --version"               "Rust corner cannot build"      1
check java    "java -version"                 "TLC and JVM corners unavailable" 1
check kotlinc "kotlinc -version"              "Kotlin corner cannot build"    0
check verus   "verus --version | grep -i ver" "Rust R4 unavailable"           0
check z3      "z3 --version"                  "Verus and Gobra unavailable"   0
check jbmc    "jbmc --version"                "Java/Kotlin bounded rung"      0
[ -f /opt/gobra/gobra.jar ] \
  && printf '  ok    %-9s %s bytes\n' gobra "$(stat -c%s /opt/gobra/gobra.jar)" \
  || printf '  ABSENT %-8s -> Go R4 unavailable\n' gobra

log "Setup finished"
if [ "$missing" -gt 0 ]; then
  echo "FATAL: $missing load-bearing tool(s) missing; the environment cannot run the rig."
  exit 1
fi
echo "Verify in a session with:  go run ./tools/cmd/matrixctl doctor"
exit 0
