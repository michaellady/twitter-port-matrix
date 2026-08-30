# Environment for the twitter_port_matrix rig.
#
# Every version here is pinned to what the findings were produced on. A
# verification result is only meaningful against a known toolchain -- F014 is
# a defect in one specific JBMC build, and F012's blockers are properties of
# one specific vstd release. An unpinned image would silently invalidate them.
#
# Gobra note: Gobra is a single fat jar whose image entrypoint is
# `java -Xss128m -jar gobra.jar`. It does NOT need a container at runtime --
# it needs a JVM and a Z3 on PATH. The image is only a distribution channel
# (viperproject/gobra publishes no release assets), so the jar is lifted out
# here at BUILD time and the runtime has no Docker dependency at all.
# Measured on the macOS host: 20s native for the store package versus ~36s
# through the amd64 image under emulation.
FROM ghcr.io/viperproject/gobra@sha256:2ef080ccd284945829501996e6d63ed2f1c94b7cf6a30d2b934272fb8a6df2c6 AS gobra

# linux/amd64 is pinned explicitly, not inherited. The Verus asset is
# x86-linux and the CBMC package is an amd64 .deb, so an unpinned base builds
# fine on an amd64 cloud host and fails on an arm64 laptop -- a difference that
# would only ever surface on someone else's machine.
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

# --- Gobra, as a plain jar --------------------------------------------
# sha256 33d2dce591af60c48e3b11af1bf7f41a31a70fb7578ecefe1728748b58f30321
# (Gobra 1.1-SNAPSHOT, from the v25.02 image digest pinned above).
# Z3 comes from the Verus bundle already on PATH; Gobra finds it there.
COPY --from=gobra /gobra/gobra.jar /opt/gobra/gobra.jar
ENV GOBRA_JAR=/opt/gobra/gobra.jar

WORKDIR /workspace
