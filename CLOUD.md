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
3. Leave network access at **Trusted** — the default allowlist covers GitHub
   release assets, crates.io and apt, which is everything the script fetches
   **except the Gobra jar**. See "Gobra does not need Docker, but it does need
   a blob host" below: `ghcr.io` itself is allowed and its blob storage is not,
   so Go R4 needs an allowlist entry the Trusted default does not include.

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

### Known failure: blocked PPAs in the base image

The first version of this script died at its first command. The base image
ships third-party PPAs — `deadsnakes`, `ondrej/php` — that the Trusted network
allowlist blocks with `403 Forbidden`, so `apt-get update` exits nonzero over
repositories this project never uses, and `set -e` took the whole install down
with it.

The script now disables any `sources.list.d` entry pointing at Launchpad before
updating, and does not use a global `set -e`. Each step is individually
tolerant and **each tool is verified by running it**, not by trusting an
installer's exit code. It exits nonzero only when something load-bearing (Go,
Rust, a JVM) is genuinely absent.

JDK 17 is now optional. The findings were produced on 17 and the base ships 21,
but nothing in them depends on the JVM version — F014 is a CBMC defect and
TLC's state count is deterministic — so the script falls back to 21 and says
which it used rather than failing.

### One risk worth knowing before you paste it

The setup field allows roughly **five minutes**. `cloud-setup.sh` downloads
Verus (~100 MB), Kotlin (~100 MB), a CBMC package, and pulls a 669 MB image to
extract a 105 MB jar. That may not fit the budget on a slow fetch.

The script is ordered so the cheapest and most load-bearing tools land first,
and the Gobra step is last and non-fatal — if the image pull fails it prints a
warning and continues, leaving the Go corner at R0–R3 while everything else
works. If the whole script times out, move the Gobra block to a session-time
step; it is the only part that can be deferred without losing a rung entirely.

**It has now been run.** Everything above landed except Gobra, and the one that
did not failed for a reason no timeout budget would have fixed — see the next
section. Measured in a real cloud session:

| tool | outcome |
|---|---|
| JDK, `unzip` | installed |
| rustc | **1.95.0**, the pinned toolchain, so Verus runs |
| Verus 0.2026.04.24.f8e1704 | present; `verify` green over all five crates |
| CBMC/JBMC 6.11.0 | installed and runnable |
| kotlinc 2.4.10 | installed |
| Gobra jar | **absent — blocked, not slow** |

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

**Now verified: the directory name inside the Verus zip.** It is
`verus-x86-linux/verus`, as inferred. `cloud-setup.sh` does not rely on the
inference either way — it `find`s the binary — and a cloud session confirmed
both: the path resolves and `cargo-verus verus verify` returns
`21 verified, 0 errors` across the five verify-enabled crates (`23` when that
session ran; S-14 deleted two contentless twins -- see `evidence/findings/F024`).
**Verus caches.** A second run over an unchanged tree prints no
`verification results::` line at all, which reads exactly like a pass. `touch`
the crate sources before every run.

**Also now verified: rustc must be 1.95.0 for any of that to happen.** The base
image ships 1.94.1, on which Verus installs cleanly and then refuses to run.
The `rustup toolchain install 1.95.0` step is not belt-and-braces; without it
Rust R4 is unreachable no matter what `doctor` says about the Verus binary.

## Gobra does not need Docker, but it does need a blob host

Everything below about the jar is still true — and it is not sufficient, which
is the part that cost a rung.

**`ghcr.io` being allowlisted is not the same as the image being fetchable.**
The registry API on `ghcr.io` answers fine: `crane manifest` on the pinned
digest returns the manifest, layers and all. Layer *contents* are served from
`pkg-containers.githubusercontent.com` via a redirect, and that host is not on
the Trusted allowlist:

```
Error: reading layer contents: Get "https://pkg-containers.githubusercontent.com/ghcr1/blobs/sha256:630b3e64...": Forbidden
```

confirmed at the proxy as `connect_rejected: gateway answered 403 to CONNECT
(policy denial or upstream failure), host pkg-containers.githubusercontent.com:443`.

So `crane export` produces a **zero-byte** file, the setup script's `[ -s ]`
test fails, and it takes the "could not be fetched" branch. Note which branch:
the sha256 pin never gets to run, so an empty `/opt/gobra/` means *blocked*,
not *tampered with*. Both branches `rm -f` and warn, and the distinction
matters enough that the script now says which one it took.

`viperproject/gobra` publishes no release assets, so there is no second URL to
try. **To get Go R4 in a cloud session, add `pkg-containers.githubusercontent.com`
to the environment's network allowlist** (Custom rather than Trusted), or
install the jar from a host that is already allowed.

The cost of not doing so is larger than one rung: R5's discharged portion is
31 of 42 clauses and **every one of them is Gobra-backed**
(`spec/refinement/obligations.json`), so a session without the jar reaches
neither R4/Go nor any of R5.

## The jar itself

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

**On a host that can fetch the jar, this reaches every ceiling the macOS host
does.** On a Trusted cloud session it does not, and that is the degraded mode
the sentence above wrongly denied. Recorded rather than corrected away, because
it is the same shape as `evidence/FINDINGS.md` Pattern 5: a claim that was true
where it was written and was inherited as a fact about the world.

## What degrades, and where it degrades to

| rung | needs | without it |
|---|---|---|
| R0, R1, R2 | Go only | — always available |
| R3 model check | JDK + the committed jar | — always available |
| R4 Rust (Verus) | the Verus binary | — available |
| R4 Kotlin/Java (JBMC) | CBMC package | — available |
| R4 Go (Gobra) | JVM + Z3 on PATH + **the jar** | **unavailable in a Trusted cloud session** — the jar's blob host is blocked; see above |
| R5 | the same jar | unavailable with it: 31 of 42 discharged clauses are Gobra-backed |

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
