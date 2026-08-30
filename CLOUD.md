# Running this rig somewhere other than the machine it was built on

## Setup

Three paths. All of them use `.devcontainer/`, which pins every tool to the
build the findings were produced on.

### 1. GitHub Codespaces

Codespaces is enabled on this repository. The `gh` CLI needs one extra scope
first — a browser flow, so it cannot be scripted for you:

```bash
gh auth refresh -h github.com -s codespace
```

```bash
gh codespace create -R michaellady/twitter-port-matrix -m standardLinux32gb
```

```bash
gh codespace ssh
```

`standardLinux32gb` (4 cores, 16 GB RAM, 32 GB storage) is the smallest size
that comfortably fits the work. RAM is the binding constraint, not cores: the
R3 model check explores 8,989,719 distinct states, and JBMC has been seen to
reach 11 GB on a nondeterministic `limit`. `basicLinux32gb` at 8 GB will run
R0–R2 and Verus but will struggle with R3 and JBMC.

Available sizes on this repo: `basicLinux32gb`, `standardLinux32gb`,
`premiumLinux`, `largePremiumLinux`.

### 2. Plain Docker, anywhere

No Codespaces, no devcontainer tooling — just the image:

```bash
git clone https://github.com/michaellady/twitter-port-matrix && cd twitter-port-matrix
```

```bash
docker build --platform linux/amd64 -f .devcontainer/Dockerfile -t tpm-env .
```

```bash
docker run -it --rm -v "$PWD":/workspace -w /workspace tpm-env bash
```

Then inside the container:

```bash
bash .devcontainer/setup.sh
```

`--platform linux/amd64` is required on an arm64 host. The Verus asset is
`x86-linux` and the CBMC package is an amd64 `.deb`, so the runtime stage pins
`--platform=linux/amd64` in the Dockerfile too — an unpinned base builds on an
amd64 cloud host and fails on an arm64 laptop.

### 3. Bare metal

If you would rather install directly, `.devcontainer/Dockerfile` is the
shopping list with every version and URL. The one non-obvious item is Gobra:
it is a fat jar inside `ghcr.io/viperproject/gobra`, which publishes no release
assets, so it has to be lifted out of the image once and then runs standalone.

## Verifying the environment is right

```bash
go run ./tools/cmd/matrixctl doctor
```

That checks every tool is present and at the pinned version, that the vendored
TLA+ spec still matches its digest, and that no implementation imports `S_obs`.
Then:

```bash
go run ./tools/cmd/matrixctl spec check
```

Four gates, roughly 90 seconds, most of it TLC. If both pass, the environment
reproduces the findings.


`.devcontainer/` describes the environment. This file says what survives the
move and what does not.

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
