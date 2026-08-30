# Running this rig somewhere other than the machine it was built on

## Setup

### Claude Code cloud sessions — the supported path

**`.devcontainer/` is not read by Claude cloud sessions.** That directory is a
local / VS Code / Codespaces convention and is kept here only for the plain
Docker path below. The cloud mechanism is a **setup script**.

1. Open the environment settings for this repo at
   [claude.ai/code](https://claude.ai/code).
2. Paste the contents of [`cloud-setup.sh`](cloud-setup.sh) into the
   **Setup script** field.
3. Leave network access at **Trusted** — the default allowlist already covers
   `ghcr.io`, GitHub release assets, crates.io and apt, which is everything the
   script fetches.

The script runs **once**, as root, on Ubuntu 24.04 x86_64. Anthropic snapshots
the filesystem afterwards, so later sessions reuse everything it installed.
Packages Claude installs *mid-session* do not persist; the repo is re-cloned
fresh each time.

`.claude/settings.json` additionally registers a `SessionStart` hook that runs
[`.claude/session-start.sh`](.claude/session-start.sh) on every session, local
and cloud. It installs nothing — it reports which rungs are reachable, so a
session that cannot run Gobra says so in its first line rather than three
commands later.

### What the base image already has, and what it does not

Pre-installed: Go, Rust (`rustc`/`cargo`), Docker with a working daemon,
**OpenJDK 21**, `git`, `gh`, `jq`, `python3`.

Installed by `cloud-setup.sh`, because the base lacks them:

| tool | why |
|---|---|
| **JDK 17** | the findings were produced on 17; the base ships 21 |
| **kotlinc 2.4.10** | absent entirely |
| **Verus 0.2026.04.24** | absent; the Rust deductive rung |
| **CBMC 6.11.0** | absent; provides `jbmc`, and F014 *is* a defect in this exact build |
| **Gobra jar** | distributed only inside a container image, lifted out at setup |

### Resource fit

4 vCPU, 16 GB RAM, 30 GB disk per session. RAM is the binding constraint and
**16 GB is enough**: the R3 model check explores 8,989,719 distinct states, and
JBMC was observed at 11 GB RSS during the Kotlin work. Both fit, with less
headroom than is comfortable for JBMC.

### One risk worth knowing before you paste it

The setup field allows roughly **five minutes**. `cloud-setup.sh` downloads
Verus (~100 MB), Kotlin (~100 MB), a CBMC package, and pulls a 669 MB image to
extract a 105 MB jar. That may not fit the budget on a slow fetch.

The script is ordered so the cheapest and most load-bearing tools land first,
and the Gobra step is last and non-fatal — if the image pull fails it prints a
warning and continues, leaving the Go corner at R0–R3 while everything else
works. If the whole script times out, move the Gobra block to a session-time
step; it is the only part that can be deferred without losing a rung entirely.

**This has not been run yet.** The script is syntax-checked and every URL in it
was probed, but no cloud session has executed it.

### Plain Docker, anywhere (not the cloud path)

```bash
git clone https://github.com/michaellady/twitter-port-matrix && cd twitter-port-matrix
```

```bash
docker build --platform linux/amd64 -f .devcontainer/Dockerfile -t tpm-env .
```

```bash
docker run -it --rm -v "$PWD":/workspace -w /workspace tpm-env bash
```

`--platform linux/amd64` is required: the Verus asset is `x86-linux` and the
CBMC package is an amd64 `.deb`.

### Verifying any environment

```bash
go run ./tools/cmd/matrixctl doctor
```

```bash
go run ./tools/cmd/matrixctl spec check
```

Four gates, roughly 90 seconds, most of it TLC. If both pass, the environment
reproduces the findings.

## Why every version is pinned

The findings are not generic. Several are properties of one specific build:

- **F014** — `"abc".equals("abc")` verifies as FALSE — is a defect in
  **CBMC/JBMC 6.11.0**. On a different build it may not reproduce, and the
  Kotlin and Java ceilings would be wrong.
- **F012** — `abs_rust` is undefinable because `RwLock` has no model — is a
  property of the **`vstd` release bundled with Verus 0.2026.04.24**.
- **F016** — 23 units, 11 empty — is a count from that same Verus.
- **R3** — 8,989,719 distinct states — is **TLC 2.19**, from a jar committed
  here with its sha256 pinned in `docker/pins.json`.
- **R4/Go** — 283 Viper members — is **Gobra v25.02**, pinned by image digest.

An unpinned image would re-run all of this against different tools and quietly
produce different numbers against the same findings. The pins are the point.

## What was verified, and what was not

Every download URL in the Dockerfile was probed and returns 200:

| | |
|---|---|
| `go1.25.5.linux-amd64.tar.gz` | 200 |
| `verus-0.2026.04.24.f8e1704-x86-linux.zip` | 200 |
| `kotlin-compiler-2.4.10.zip` | 200 |
| `ubuntu-24.04-cbmc-6.11.0-Linux.deb` | 200 |

The Verus URL took two attempts: the asset is `x86-linux`, not `x86_64-linux`,
and the version string carries the commit hash (`0.2026.04.24.f8e1704`). The
short form 404s.

**Not verified: the directory name inside the Verus zip.** `VERUS_PATH` assumes
`verus-x86-linux/verus`, inferred from the macOS archive unpacking to
`verus-arm64-macos/`. If that inference is wrong the image builds and
`matrixctl doctor` reports Verus absent — a loud failure, not a silent one, and
`VERUS_PATH` is an env var precisely so it can be corrected without a rebuild.

## Gobra does not need Docker

Gobra is a single 105 MB fat jar. The image entrypoint is literally
`java -Xss128m -jar gobra.jar`, so it needs a JVM and a Z3 on `PATH` — not a
container. Verified on the macOS host with no daemon involved:

```
Verifying package internal/store - store
Gobra found no errors
Gobra has found 0 error(s)          20.1s
```

against roughly 36s through the amd64 image under emulation on arm64.

`viperproject/gobra` publishes no release assets, so the image is the only
distribution channel — but that is a *build-time* concern. The Dockerfile lifts
the jar out with a multi-stage `COPY --from`, and the runtime has no Docker
dependency at all.

**Consequence: this environment reaches every ceiling the macOS host does.**
There is no degraded mode.

## What degrades, and where it degrades to

| rung | needs | without it |
|---|---|---|
| R0, R1, R2 | Go only | — always available |
| R3 model check | JDK + the committed jar | — always available |
| R4 Rust (Verus) | the Verus binary | — available |
| R4 Kotlin/Java (JBMC) | CBMC package | — available |
| R4 Go (Gobra) | JVM + Z3 on PATH | — available |

**Nothing needs a Docker daemon at runtime.** That was the original design and
it was wrong: it cost an unnecessary `docker-in-docker` feature, and it made
Gobra roughly twice as slow on this host by running an amd64 image under
emulation.

## The host-specific thing that was fixed

`matrixctl doctor` had `/Users/mikelady/.verus/...` hardcoded, which made the
repository unrunnable anywhere else. It now reads `VERUS_PATH`, falls back to
`verus` on `PATH`, and only then to the macOS default.

## The blocker

**This repository has no remote, deliberately.** That was an explicit choice at
the start: local-only, no `.github/`, nothing pushed, with the four source
repositories consumed read-only and left untouched.

A cloud agent cannot check out a repository that exists only on this disk. So
running this in the cloud requires pushing it somewhere — which reverses that
decision and publishes the work, including vendored copies of four private
repositories' source.

That is a call for the owner, not an inference from "set up a cloud agent."
