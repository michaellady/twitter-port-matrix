# Environment for the twitter_port_matrix rig.
#
# Every version here is pinned to what the findings were produced on. A
# verification result is only meaningful against a known toolchain -- F014 is
# a defect in one specific JBMC build, and F012's blockers are properties of
# one specific vstd release. An unpinned image would silently invalidate them.
#
# Platform note: this is linux/amd64 on purpose. The Gobra image is amd64-only
# and has been running under emulation on the macOS host; here it is native.
FROM --platform=linux/amd64 ubuntu:24.04

ARG GO_VERSION=1.25.5
ARG RUST_VERSION=1.95.0
# Full version INCLUDING the commit hash -- the release tag and the asset
# name both carry it, and the guessed short form 404s.
ARG VERUS_VERSION=0.2026.04.24.f8e1704
ARG KOTLIN_VERSION=2.4.10
ARG CBMC_VERSION=6.11.0

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git unzip zip build-essential pkg-config \
      openjdk-17-jdk python3 jq \
    && rm -rf /var/lib/apt/lists/*

# --- Go -----------------------------------------------------------------
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
      | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"

# --- Rust ---------------------------------------------------------------
RUN curl -fsSL https://sh.rustup.rs | sh -s -- -y \
      --default-toolchain "${RUST_VERSION}" --profile minimal --component clippy
ENV PATH="/root/.cargo/bin:${PATH}"

# --- Verus + its bundled Z3 ---------------------------------------------
# The macOS host runs verus-arm64-macos; this is the x86_64-linux build of the
# same release. VERUS_PATH is what tools/cmd/matrixctl/doctor.go reads.
RUN mkdir -p /opt/verus && cd /opt/verus \
    && curl -fsSL -o verus.zip \
       "https://github.com/verus-lang/verus/releases/download/release%2F${VERUS_VERSION}/verus-${VERUS_VERSION}-x86-linux.zip" \
    && unzip -q verus.zip && rm verus.zip
ENV VERUS_PATH=/opt/verus/verus-x86-linux/verus
ENV PATH="/opt/verus/verus-x86-linux:${PATH}"

# --- Kotlin -------------------------------------------------------------
RUN curl -fsSL -o /tmp/kotlin.zip \
      "https://github.com/JetBrains/kotlin/releases/download/v${KOTLIN_VERSION}/kotlin-compiler-${KOTLIN_VERSION}.zip" \
    && unzip -q /tmp/kotlin.zip -d /opt && rm /tmp/kotlin.zip
ENV PATH="/opt/kotlinc/bin:${PATH}"

# --- CBMC / JBMC --------------------------------------------------------
# F014 is a defect in THIS build. Pin it, or that finding stops reproducing.
RUN curl -fsSL -o /tmp/cbmc.deb \
      "https://github.com/diffblue/cbmc/releases/download/cbmc-${CBMC_VERSION}/ubuntu-24.04-cbmc-${CBMC_VERSION}-Linux.deb" \
    && apt-get update && apt-get install -y --no-install-recommends /tmp/cbmc.deb \
    && rm /tmp/cbmc.deb && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
