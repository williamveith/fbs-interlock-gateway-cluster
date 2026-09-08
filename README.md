---
title: "FBS Interlock Gateway Cluster"
subtitle: "Project Overview and Operations Reference"
author: "William Veith"
lang: en-US
---

> **Purpose**
>
> This README is the authoritative project-level reference for `fbs-interlock-gateway-cluster`. It documents the repository architecture, container assembly, Docker Swarm deployment, Litestream/Cloudflare R2 persistence, Docker Secrets, physical-node networking, uplink failover, development, validation, CI, release publication, recovery, and operational Make targets.
>
> **Documentation durability**
>
> This README intentionally does **not** duplicate volatile release numbers, image tags, base-image versions, port ranges, replica counts, UID/GID values, bucket names, timeouts, or other configuration values that already have an authoritative definition in source. Those values change over time. Each section identifies the file or Make variable that owns the live value.
>
> **Project boundary**
>
> This repository does **not** contain the FBS Interlock Gateway application implementation. Gateway request handling, Shelly communication, configuration schema, Admin UI/API behavior, SQLite schema, and application-level security are maintained in [`williamveith/fbs-interlock-gateway`](https://github.com/williamveith/fbs-interlock-gateway). This repository consumes a pinned official gateway release and provides the cluster/deployment layer around it.

# Table of Contents

- [Project Overview](#project-overview)
- [Sources of Truth](#sources-of-truth)
- [Responsibility Split](#responsibility-split)
- [Capabilities](#capabilities)
- [System Architecture](#system-architecture)
  - [Runtime Startup Flow](#runtime-startup-flow)
  - [Persistent-State Flow](#persistent-state-flow)
  - [Swarm Failover Model](#swarm-failover-model)
- [Repository Layout](#repository-layout)
- [Version and Dependency Model](#version-and-dependency-model)
- [Container Image](#container-image)
  - [Image Construction](#image-construction)
  - [Upstream Gateway Verification](#upstream-gateway-verification)
  - [Swarm Entrypoint](#swarm-entrypoint)
  - [Runtime Identity](#runtime-identity)
- [Litestream and Cloudflare R2](#litestream-and-cloudflare-r2)
  - [Database Paths](#database-paths)
  - [Restore Behavior](#restore-behavior)
  - [First Deployment Without an Existing Replica](#first-deployment-without-an-existing-replica)
- [Docker Swarm Stack](#docker-swarm-stack)
  - [Image Selection](#image-selection)
  - [Published Ports](#published-ports)
  - [Single-Replica Service Model](#single-replica-service-model)
  - [External Volume](#external-volume)
  - [Docker Secrets](#docker-secrets)
- [Prerequisites and Supported Platforms](#prerequisites-and-supported-platforms)
- [Required Local Configuration](#required-local-configuration)
  - [R2 Environment File](#r2-environment-file)
  - [Runtime TLS Files](#runtime-tls-files)
- [Development and Validation](#development-and-validation)
  - [Unified Validation Gate](#unified-validation-gate)
  - [Container Smoke Test](#container-smoke-test)
  - [Local Container Build](#local-container-build)
- [Container Registry Publication](#container-registry-publication)
- [Swarm Deployment](#swarm-deployment)
  - [Preflight](#preflight)
  - [Normal Deployment](#normal-deployment)
  - [Destructive Restore Test](#destructive-restore-test)
  - [Full Local Swarm Test](#full-local-swarm-test)
  - [Status and Logs](#status-and-logs)
- [Physical Multi-Node Cluster](#physical-multi-node-cluster)
  - [Private Swarm Network](#private-swarm-network)
  - [Initialize the First Manager](#initialize-the-first-manager)
  - [Join Additional Managers](#join-additional-managers)
  - [Provision the Local Data Volume on Every Eligible Node](#provision-the-local-data-volume-on-every-eligible-node)
  - [Shared Uplink Identity](#shared-uplink-identity)
  - [Uplink Controller](#uplink-controller)
- [Recovery and Failure Behavior](#recovery-and-failure-behavior)
- [Makefile Reference](#makefile-reference)
  - [Core Variables](#core-variables)
  - [Development and Validation Targets](#development-and-validation-targets)
  - [Container Targets](#container-targets)
  - [Swarm Targets](#swarm-targets)
  - [Physical Node Targets](#physical-node-targets)
- [Continuous Integration](#continuous-integration)
- [Release Workflow](#release-workflow)
  - [Release Environment](#release-environment)
  - [Release Stages](#release-stages)
  - [Release Metadata](#release-metadata)
  - [Immutable Releases](#immutable-releases)
  - [Release Verification](#release-verification)
- [Updating the Upstream Gateway](#updating-the-upstream-gateway)
- [Releasing a New Cluster Version](#releasing-a-new-cluster-version)
- [Branch and Pull-Request Workflow](#branch-and-pull-request-workflow)
- [Security Model](#security-model)
- [Troubleshooting](#troubleshooting)
- [Repository Safety](#repository-safety)
- [License](#license)

# Project Overview

`fbs-interlock-gateway-cluster` is the Docker Swarm deployment and high-availability infrastructure layer for the [FBS Interlock Gateway](https://github.com/williamveith/fbs-interlock-gateway).

The cluster repository packages a **pinned official gateway release** into a Linux container alongside Litestream and a small cluster-specific startup helper. Docker Swarm runs one active gateway task, while Litestream protects the gateway's authoritative SQLite database in Cloudflare R2.

The repository also contains host-level tooling for physical cluster nodes, including:

- private Swarm networking
- explicit Swarm manager addressing
- node join/bootstrap operations
- local external-volume provisioning
- shared uplink identity configuration
- a systemd-managed uplink controller
- Docker Swarm task rescheduling
- R2-backed database recovery on an empty replacement node

The published image repository is defined by `DOCKERHUB_IMAGE` in the root `Makefile`.

The cluster source repository is:

```text
https://github.com/williamveith/fbs-interlock-gateway-cluster
```

The gateway application source repository is:

```text
https://github.com/williamveith/fbs-interlock-gateway
```

# Sources of Truth

When documentation and source disagree, **the source file listed here is authoritative**.

| Setting or behavior | Authoritative source |
| --- | --- |
| Upstream gateway release consumed by the image | `Makefile` → `GATEWAY_VERSION` |
| Cluster Docker Hub image tag | `Makefile` → `DOCKERHUB_TAG` |
| Docker Hub repository | `Makefile` → `DOCKERHUB_IMAGE` |
| GitHub Container Registry repository | `.github/workflows/release.yml` → repository-derived `ghcr.io` image |
| Supported published container platforms | `Makefile` → `DOCKERHUB_PLATFORMS` |
| Registry mirroring, digest equality, and `latest` aliases | `.github/workflows/release.yml` |
| Local container image/tag/platform | `Makefile` → `CONTAINER_*` |
| Swarm image deployed | `Makefile` → `SWARM_IMAGE` |
| Swarm stack/service/volume names | `Makefile` → `SWARM_*` |
| Swarm runtime UID/GID | `Makefile` → `SWARM_DATA_UID`, `SWARM_DATA_GID` |
| Swarm secret names | `Makefile` and `cluster/swarm/stack.yml` |
| Runtime TLS source paths | `Makefile` → `TLS_*_SOURCE` |
| Litestream base image/version | `Dockerfile` |
| Go builder/base image versions | `Dockerfile` |
| Upstream gateway download/verification logic | `Dockerfile` |
| Gateway runtime command | `litestream.yml` |
| R2 bucket/path/endpoint and replication settings | `litestream.yml` |
| Published ports | `cluster/swarm/stack.yml` |
| Replica count, restart/update/rollback behavior | `cluster/swarm/stack.yml` |
| Docker Secret mount paths and modes | `cluster/swarm/stack.yml` |
| Physical-node variables and Make targets | `cluster/node/Makefile.mk` |
| Private-network configuration behavior | `scripts/cluster/configure-private-network.sh` |
| Uplink configuration behavior | `scripts/cluster/configure-uplink.sh` |
| Uplink controller behavior | `scripts/cluster/fbs-swarm-uplink.sh` |
| Uplink systemd service | `services/linux/fbs-swarm-uplink.service` |
| CI behavior | `.github/workflows/ci.yml` |
| Release behavior | `.github/workflows/release.yml` |
| Dependency update schedule | `.github/dependabot.yml` |

This design is deliberate: the README explains **what a setting means and how the system fits together**, while source code owns the current value.

Useful commands when reviewing the live configuration:

```bash
grep -E '^(GATEWAY_VERSION|DOCKERHUB_IMAGE|DOCKERHUB_TAG|DOCKERHUB_PLATFORMS|SWARM_IMAGE)' Makefile
```

```bash
cat litestream.yml
```

```bash
cat cluster/swarm/stack.yml
```

```bash
grep '^FROM ' Dockerfile
```

```bash
grep -nE 'ghcr|packages: write|imagetools create' .github/workflows/release.yml
```

# Responsibility Split

The standalone gateway repository owns application behavior. The cluster repository owns deployment infrastructure.

| Component | Owner |
| --- | --- |
| FBS listener behavior | `fbs-interlock-gateway` |
| `/status`, `/on`, `/off` request handling | `fbs-interlock-gateway` |
| Shelly HTTP/HTTPS RPC | `fbs-interlock-gateway` |
| Digest Authentication | `fbs-interlock-gateway` |
| Shelly mutual TLS application behavior | `fbs-interlock-gateway` |
| Gateway Admin UI/API | `fbs-interlock-gateway` |
| SQLite configuration schema and migrations | `fbs-interlock-gateway` |
| Gateway release binaries | `fbs-interlock-gateway` |
| Container image assembly | `fbs-interlock-gateway-cluster` |
| `swarm-entrypoint` | `fbs-interlock-gateway-cluster` |
| Litestream configuration | `fbs-interlock-gateway-cluster` |
| Cloudflare R2 replication | `fbs-interlock-gateway-cluster` |
| Docker Swarm stack | `fbs-interlock-gateway-cluster` |
| Swarm Docker Secrets | `fbs-interlock-gateway-cluster` |
| Physical-node private networking | `fbs-interlock-gateway-cluster` |
| Shared uplink failover | `fbs-interlock-gateway-cluster` |
| Multi-platform Docker publication | `fbs-interlock-gateway-cluster` |

Gateway application fixes should be made upstream and consumed here through a new pinned gateway release. Do not copy the gateway implementation back into this repository.

# Capabilities

The cluster project provides:

- multi-platform Linux container publication
- a pinned official `fbs-interlock-gateway` release inside the image
- SHA-256 verification of the downloaded upstream gateway asset during image construction
- a small cluster-specific Go entrypoint
- Litestream-backed SQLite replication
- Cloudflare R2 persistence
- automatic database restore before gateway startup when the local database is absent
- Docker Swarm deployment with one active gateway service replica
- controlled service restart/update/rollback behavior
- an external Docker volume for `/data`
- Docker Secrets for R2 credentials
- Docker Secrets for gateway runtime TLS files
- non-root runtime execution
- `make verify` as the repository validation gate
- `make container-smoke` as the container integration test
- destructive R2 recovery testing through `make swarm-reset-deploy`
- full local Swarm purge/rebuild testing through `make swarm-full-test`
- private-network bootstrap for physical Swarm managers
- shared uplink identity configuration
- a systemd-managed uplink controller
- dual publication to Docker Hub and GitHub Container Registry (GHCR)
- one multi-platform build mirrored to GHCR as the same OCI image index
- BuildKit provenance and SBOM attestations preserved across both registries
- release-time verification that Docker Hub and GHCR resolve to the same OCI index digest
- GPG-signed Git release tags
- immutable GitHub Releases
- release metadata tying a GitHub release to an exact image digest and upstream gateway release
- Dependabot maintenance for supported dependency ecosystems

Exact platform lists, versions, tags, and infrastructure values are intentionally read from their source files rather than repeated here.

# System Architecture

```text
                         Docker Swarm cluster
                  +-------------------------------+
                  |                               |
FBS ------------> | published gateway ports       |
                  |           |                   |
                  |           v                   |
                  |  +-------------------------+  |
                  |  | active gateway task     |  |
                  |  |                         |  |
                  |  | swarm-entrypoint        |  |
                  |  |        |                |  |
                  |  |        v                |  |
                  |  |    Litestream           |  |
                  |  |        |                |  |
                  |  |        v                |  |
                  |  | fbs-interlock-gateway   |  |
                  |  |        |                |  |
                  |  +--------|----------------+  |
                  |           |                   |
                  +-----------|-------------------+
                              |
                              +------> Shelly interlocks
                              |
                              v
                     /data/gateway.sqlite3
                              |
                              v
                          Litestream
                              |
                              v
                        Cloudflare R2
```

Only one gateway replica is intended to be active. The Swarm stack owns the exact replica count and update policy.

## Runtime Startup Flow

The final image starts with:

```text
/swarm-entrypoint
```

The entrypoint:

1. checks whether the R2 access-key environment variable already exists
2. otherwise reads the corresponding Docker Secret file
3. checks whether the R2 secret-key environment variable already exists
4. otherwise reads the corresponding Docker Secret file
5. exports those values into the process environment
6. locates Litestream
7. replaces itself with Litestream using `exec`
8. Litestream reads `/etc/litestream.yml`
9. Litestream applies its configured restore behavior
10. Litestream starts the configured gateway command
11. Litestream continues database replication while the gateway runs

The exact environment-variable names and Secret paths are defined in `cmd/swarm-entrypoint/main.go`.

The exact gateway command is defined in `litestream.yml`.

## Persistent-State Flow

```text
gateway configuration changes
          |
          v
/data/gateway.sqlite3
          |
          v
      Litestream
          |
          v
Cloudflare R2
```

The exact R2 bucket, path, endpoint, sync settings, and restore options are defined in `litestream.yml`.

Recovery is the reverse:

```text
replacement task
      |
      v
local /data
      |
      +--> database exists -> gateway uses local state
      |
      +--> database absent -> Litestream applies configured restore behavior
                              |
                              v
                         gateway starts
```

## Swarm Failover Model

The stack is designed around a **single active gateway task**, not active-active gateway replicas.

Conceptually:

```text
active task fails
      |
      v
Swarm schedules replacement task
      |
      v
replacement task opens node-local /data
      |
      +--> existing database -> use it
      |
      +--> empty database path -> restore path handled by Litestream
```

The physical-node uplink controller is separate from Swarm task scheduling. Swarm decides where the service task runs; the host uplink tooling manages the external network identity intended for the active service path.

# Repository Layout

The repository is intentionally limited to cluster/deployment concerns:

```text
.
├── .github/
│   ├── dependabot.yml
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── Dockerfile
├── LICENSE.md
├── Makefile
├── README.md
├── cluster
│   ├── node
│   │   ├── Makefile.mk
│   │   └── README.md
│   └── swarm
│       └── stack.yml
├── cmd
│   └── swarm-entrypoint
│       └── main.go
├── go.mod
├── go.sum
├── litestream.yml
├── pki
│   ├── ca
│   │   └── server-ca.crt
│   └── gateway
│       ├── gateway-client.crt
│       └── gateway-client.key
├── scripts
│   └── cluster
│       ├── configure-private-network.sh
│       ├── configure-uplink.sh
│       ├── fbs-swarm-uplink.sh
│       └── install-uplink-controller.sh
└── services
    └── linux
        └── fbs-swarm-uplink.service
```

Key ownership:

```text
Dockerfile
  builds the cluster helper, obtains the pinned gateway release,
  verifies it, and assembles the final image

cmd/swarm-entrypoint/
  loads Swarm-provided R2 credentials and execs Litestream

litestream.yml
  owns database restore/replication settings and the gateway runtime command

cluster/swarm/stack.yml
  owns the Swarm service, image interpolation, ports, volume,
  secret mounts, and deploy policy

cluster/node/Makefile.mk
  owns host-level node bootstrap/join/uplink Make targets

scripts/cluster/
  implements host networking and uplink behavior

services/linux/
  contains the uplink-controller systemd service

.github/workflows/
  owns CI and protected release automation

.github/dependabot.yml
  owns dependency-update scheduling
```

There is intentionally no duplicated gateway `internal/` tree and no cluster copy of the gateway application command.

# Version and Dependency Model

The cluster version and gateway version are independent.

The live values are owned by:

```text
Makefile:
  GATEWAY_VERSION
  DOCKERHUB_TAG
  DOCKERHUB_PLATFORMS

Dockerfile:
  builder/base image versions
  Litestream image version
```

Do not infer the current embedded gateway version from this README.

To see the configured upstream gateway release:

```bash
grep '^GATEWAY_VERSION' Makefile
```

To see the configured cluster Docker Hub tag:

```bash
grep '^DOCKERHUB_TAG' Makefile
```

To see the published platforms:

```bash
grep '^DOCKERHUB_PLATFORMS' Makefile
```

To see the Litestream/base-image pins:

```bash
grep '^FROM ' Dockerfile
```

An existing published cluster tag should remain associated with the dependency set used to build it. Do not rebuild the same published cluster version with a different upstream gateway release and treat the result as equivalent.

# Container Image

## Image Construction

The Dockerfile has three conceptual stages.

### 1. Build `swarm-entrypoint`

The build stage compiles only:

```text
./cmd/swarm-entrypoint
```

for the requested Linux target.

The exact Go builder image and compiler flags are defined in `Dockerfile`.

### 2. Obtain the official gateway release

The gateway stage downloads the architecture-specific official release asset from:

```text
williamveith/fbs-interlock-gateway
```

The selected release is supplied through the `GATEWAY_VERSION` build argument, which the Makefile derives from its pinned `GATEWAY_VERSION` value.

The Dockerfile validates supported target OS/architecture combinations and fails unsupported builds.

### 3. Assemble the final runtime

The final stage uses the Litestream scratch image pinned in `Dockerfile`.

It copies:

```text
/fbs-interlock-gateway
/swarm-entrypoint
/etc/litestream.yml
```

into the final runtime around the Litestream executable supplied by the base image.

The image entrypoint is:

```text
/swarm-entrypoint
```

and the default command requests Litestream replication.

The final image does not require a normal shell.

The image also records build/source metadata through labels. The exact labels and build arguments are defined in `Dockerfile`.

The upstream gateway executable retains the metadata embedded by the upstream gateway release process. The cluster build does not restamp the gateway binary.

## Upstream Gateway Verification

The Dockerfile downloads:

```text
gateway release binary
matching SHA-256 file
```

and validates the binary against that checksum before copying it into the final runtime.

This protects the image build from accepting a gateway binary that does not match the selected release's published checksum file.

The protected cluster release workflow also validates the pinned upstream GitHub Release before cluster publication.

These are separate checks:

```text
Dockerfile:
  binary matches selected release checksum

Release workflow:
  selected upstream GitHub Release passes the configured release-verification step
```

The exact commands are authoritative in `Dockerfile` and `.github/workflows/release.yml`.

## Swarm Entrypoint

`cmd/swarm-entrypoint/main.go` bridges Docker Secrets and Litestream.

Production Swarm deployment stores the R2 access key and R2 secret key as Docker Secrets. Litestream expects those credentials as environment variables.

The entrypoint therefore:

```text
Docker Secret files
      |
      v
environment variables
      |
      v
Litestream
```

If the required environment variables are already populated, the helper preserves them. That behavior is used by local smoke testing.

After loading credentials, the helper replaces itself with Litestream rather than spawning a long-lived wrapper process.

## Runtime Identity

The stack runs the container under the numeric user/group configured in `cluster/swarm/stack.yml` and coordinated with the Makefile's `SWARM_DATA_UID` / `SWARM_DATA_GID` defaults.

The root Makefile prepares the external data volume with matching ownership.

Secret file ownership/mode is defined in `cluster/swarm/stack.yml`.

Do not copy the current numeric UID/GID or file modes from this README into automation; use the source files.

# Litestream and Cloudflare R2

The cluster uses Litestream to protect:

```text
/data/gateway.sqlite3
```

The exact replica configuration is authoritative in:

```text
litestream.yml
```

That file owns:

- database path
- restore behavior
- replica type
- Cloudflare R2 bucket
- R2 object path
- endpoint format
- region
- synchronization settings
- gateway process command

## Database Paths

The persistent gateway database is stored under:

```text
/data
```

The stack mounts the external data volume at that location.

Runtime TLS material is mounted separately under the path defined by `cluster/swarm/stack.yml`.

The gateway process receives its config/database arguments from `litestream.yml`.

Because those values can change as the deployment evolves, consult `litestream.yml` for the exact current command instead of relying on a duplicated command line here.

## Restore Behavior

Litestream's current restore behavior is defined in `litestream.yml`.

The design goal is:

```text
local database exists
  -> use local state

local database absent and remote replica is usable
  -> restore before normal gateway operation
```

The intentional destructive recovery test is:

```bash
make swarm-reset-deploy
```

Then inspect:

```bash
make swarm-logs
```

Do not depend on exact log wording in this README. The success criteria are:

- the service reaches its configured running replica count
- the gateway starts successfully
- Litestream begins/continues replication
- required gateway state is present after recovery

## First Deployment Without an Existing Replica

The R2 restore path only helps when a recoverable remote database already exists.

For a brand-new deployment, the gateway still requires whatever first-run seed/configuration the **upstream gateway version** expects.

The cluster mounts `/data` and launches the upstream gateway; it does not redefine the gateway's first-run configuration rules.

For those rules, use the upstream gateway README corresponding to the pinned `GATEWAY_VERSION`.

# Docker Swarm Stack

The stack is defined at:

```text
cluster/swarm/stack.yml
```

## Image Selection

The stack uses variable interpolation for its image:

```yaml
image: "${SWARM_IMAGE}"
```

The Makefile supplies `SWARM_IMAGE` during deployment.

`SWARM_IMAGE` is derived from the Docker Hub image repository and configured release tag, so the Makefile remains the deployment version source of truth.

This prevents the stack file from independently hardcoding a cluster release.

## Published Ports

The current published ports are defined only in:

```text
cluster/swarm/stack.yml
```

The stack exposes:

- the configured FBS-facing listener range used by this cluster deployment
- the configured Admin/management port

Do not copy port values from old documentation or prior releases. Inspect the current stack before changing firewall, routing, or FBS configuration.

```bash
grep -A20 'ports:' cluster/swarm/stack.yml
```

If gateway configuration expands outside the published listener range, the stack must be updated accordingly.

Docker Swarm published ports may be exposed through the routing mesh depending on the current stack configuration. Treat the stack file as part of the network security boundary.

## Single-Replica Service Model

The design expects one active gateway instance.

The authoritative values for:

- replica count
- restart policy
- update order
- update parallelism
- failure action
- monitoring window
- rollback order
- rollback parallelism
- stop grace period

are all in:

```text
cluster/swarm/stack.yml
```

The operational intent is to avoid overlapping active gateway instances during service replacement.

## External Volume

The stack declares its data volume as external.

The exact volume name is derived from the stack/Makefile configuration and is authoritative in:

```text
Makefile
cluster/swarm/stack.yml
```

Create/prepare it with:

```bash
make swarm-volume
```

The volume is node-local. It is **not** a cluster-wide shared filesystem.

For a multi-node cluster, create the external volume on every node that is eligible to run the gateway task.

R2 is what allows an otherwise empty eligible node to recover gateway database state.

## Docker Secrets

The stack consumes external secrets for two roles:

### R2 credentials

- R2 access key
- R2 secret key

### Gateway runtime TLS

- server CA trust material
- gateway client certificate
- gateway client private key

The exact secret names, mount targets, UID/GID ownership, and modes are defined in:

```text
Makefile
cluster/swarm/stack.yml
```

Do not treat README examples as authoritative secret identifiers.

# Prerequisites and Supported Platforms

## Container Platforms

The published platform list is defined by:

```make
DOCKERHUB_PLATFORMS
```

in the Makefile.

The project publishes Linux containers. Docker Desktop on other host operating systems may run one of those Linux variants in Linux-container mode, but the cluster does not publish native macOS application containers.

Check the current list with:

```bash
grep '^DOCKERHUB_PLATFORMS' Makefile
```

## Development Host

Normal development/validation requires:

- Git
- Go matching `go.mod`
- Docker Engine or Docker Desktop
- Docker Buildx
- Bash
- `file`
- ShellCheck

Staticcheck is invoked through the Go tool dependency recorded in the module rather than requiring an arbitrary global Staticcheck installation.

GitHub CLI (`gh`) is useful for release operations and verification.

## Swarm Host

A deployment manager needs:

- Docker Engine with Swarm support
- registry/network access to the configured published image
- network access to the configured R2 endpoint
- required R2 configuration
- required runtime TLS source files
- the external data volume on each eligible node

## Physical Nodes

The host-level node tooling expects Linux systems with:

- Docker Engine
- NetworkManager / `nmcli`
- systemd
- Bash
- `sudo`
- a connection/profile for the private Swarm network
- a connection/profile for the external uplink

The detailed node design belongs in:

```text
cluster/node/README.md
```

# Required Local Configuration

## R2 Environment File

The environment file path is controlled by:

```make
ENV_FILE
```

The preflight logic requires the R2 values referenced by the Makefile and `litestream.yml`.

Review the exact variable names in:

```bash
grep -n 'R2_' Makefile litestream.yml
```

The Makefile converts sensitive credential values into Docker Secrets. Non-secret endpoint/account configuration is passed to the service environment as defined by the current stack.

## Runtime TLS Files

The required TLS source paths are defined by:

```make
TLS_SERVER_CA_SOURCE
TLS_CLIENT_CERT_SOURCE
TLS_CLIENT_KEY_SOURCE
```

in the Makefile.

`make swarm-preflight` checks that those source files exist before deployment.

The Makefile then creates the corresponding external Docker Secrets if they do not already exist.

# Development and Validation

The cluster Go module is intentionally small. It primarily exists to build/test `swarm-entrypoint` and to pin tooling used by repository validation.

## Unified Validation Gate

Run:

```bash
make verify
```

The current validation composition is defined by the `verify` target in the Makefile.

It includes repository checks such as:

- Go formatting verification
- module tidy consistency
- `go vet`
- Staticcheck
- race-enabled Go tests
- Bash syntax validation
- ShellCheck
- architecture-specific `swarm-entrypoint` builds

Do not maintain a duplicate numbered list of every sub-target here. If the validation gate changes, `Makefile: verify` is authoritative.

Inspect it with:

```bash
grep -A15 '^verify:' Makefile
```

Useful individual commands include:

```bash
make fmt
make tidy
make test
make fmt-check
make tidy-check
make vet
make staticcheck
make test-race
make scripts-check
make shellcheck
make build
make build-check
```

Only targets that exist in the current Makefile should be used.

## Container Smoke Test

Run:

```bash
make container-smoke
```

This is the main local integration test for the cluster image.

The target currently verifies the essential dependency chain:

```text
build image
  |
  +-> obtain/verify pinned upstream gateway
  |
  +-> build swarm-entrypoint
  |
  +-> execute gateway version command
  |
  +-> confirm it reports GATEWAY_VERSION
  |
  +-> execute image through swarm-entrypoint
  |
  +-> confirm Litestream starts successfully
```

Expected output should be interpreted **symbolically**, not as a fixed version string. For example:

```text
fbs-interlock-gateway version=<configured-gateway-version> ...
Verified gateway <configured-gateway-version>.
...
Verified swarm-entrypoint and Litestream.
Container smoke tests passed.
```

The `<configured-gateway-version>` value is whatever `GATEWAY_VERSION` currently specifies.

Do not update this README merely because that version changes.

## Local Container Build

Build the local development image:

```bash
make container-build
```

The live image name, tag, and platform are controlled by:

```make
CONTAINER_IMAGE
CONTAINER_TAG
CONTAINER_PLATFORM
```

Override a value at invocation time when needed:

```bash
make container-build CONTAINER_PLATFORM=<platform>
```

# Container Registry Publication

Official cluster releases are distributed through two OCI registries:

```text
Docker Hub
GitHub Container Registry (GHCR)
```

Docker Hub remains the registry used by the root Makefile for normal image publication and Swarm deployment. GHCR is published by the protected release workflow as a GitHub-native mirror of the **same OCI image index**.

## Docker Hub

The Docker Hub repository, tag, and published platform list are controlled by:

```make
DOCKERHUB_IMAGE
DOCKERHUB_TAG
DOCKERHUB_PLATFORMS
```

in the root Makefile.

Local/manual publication continues to use:

```bash
make container-builder
make container-login
make container-publish
```

The exact Buildx flags are authoritative in the Makefile. The publication target currently owns:

- multi-platform image construction
- direct Docker Hub push
- BuildKit provenance attestation
- SBOM attestation
- post-publish manifest inspection

Inspect the configured Docker Hub image with:

```bash
make container-inspect
```

## GitHub Container Registry

GHCR publication occurs only in the protected GitHub Actions release workflow:

```text
.github/workflows/release.yml
```

The GHCR repository is derived from `GITHUB_REPOSITORY` rather than duplicated as another versioned Makefile setting.

Conceptually:

```text
ghcr.io/<repository-owner>/<repository-name>:<docker-tag>
```

The release job authenticates to GHCR with the workflow's built-in GitHub token and requires:

```yaml
permissions:
  packages: write
```

No separate long-lived GHCR password or personal access token is required by the current workflow.

## Build Once, Publish Twice

The release workflow does **not** perform a second container build for GHCR.

The publication path is:

```text
source commit
    |
    v
Buildx multi-platform build
    |
    v
Docker Hub OCI image index
    |
    +--> platform image manifests
    |
    +--> provenance attestations
    |
    +--> SBOM attestations
    |
    v
docker buildx imagetools create
    |
    v
GHCR OCI image index
```

The workflow copies the already-published Docker Hub OCI index into GHCR with `docker buildx imagetools create`.

This is important because both registries then represent the same built artifact rather than two independently produced builds.

The copied index includes the platform manifests and attached OCI attestation manifests generated by the original Buildx publication.

## Registry Digest Equality

After publication, the release workflow inspects both registries and extracts each top-level OCI image-index digest.

The release fails if:

```text
Docker Hub digest != GHCR digest
```

The expected invariant is:

```text
Docker Hub tag
      |
      +--> sha256:<same-index-digest>
      |
GHCR tag
```

This gives the release process a simple cross-registry identity check: both registry references must resolve to the same multi-platform OCI index.

## Stable `latest` Alias

For release versions that the workflow classifies as stable, both registries receive a `latest` alias.

Conceptually:

```text
Docker Hub:
  <repository>:<release-tag>
  <repository>:latest

GHCR:
  <repository>:<release-tag>
  <repository>:latest
```

Prereleases leave the existing `latest` aliases unchanged.

The exact stable/prerelease test and alias commands are authoritative in `.github/workflows/release.yml`.

## Deployment Registry

Swarm deployment remains controlled by:

```make
SWARM_IMAGE
```

in the root Makefile.

The addition of GHCR does not automatically change the registry used by the running Swarm deployment. The cluster can continue pulling the Docker Hub image while GHCR provides an independently hosted GitHub-native reference to the same release artifact.

To move Swarm to another registry in the future, change the source of `SWARM_IMAGE`; do not hardcode a registry in `cluster/swarm/stack.yml`.

# Swarm Deployment

## Preflight

Run:

```bash
make swarm-preflight
```

The target validates the current deployment prerequisites defined in the Makefile, including:

- Docker availability
- Docker Engine availability
- environment file presence
- required R2 configuration
- stack file presence
- required runtime TLS source files

The Makefile is authoritative for the exact checks.

## Normal Deployment

Run:

```bash
make swarm-local-deploy
```

The target composes the normal deployment sequence from lower-level Swarm targets.

At a high level it:

```text
checks prerequisites
pulls configured published image
initializes/checks Swarm
creates/checks local volume
creates/checks secrets
deploys stack
waits for service readiness
```

The exact sequence is authoritative in the Makefile.

The normal deployment path preserves an existing local data volume and existing secrets where the lower-level targets are designed to do so.

The deployment uses the **published image selected by `SWARM_IMAGE`**, not the locally built development image.

## Destructive Restore Test

> **Destructive local operation:** this target intentionally recreates local persistent state. Use it only when that reset is intended.

Run:

```bash
make swarm-reset-deploy
```

This target is designed to prove that the published cluster image can recover from an empty local data volume using the configured persistence model.

The exact reset sequence is defined in the Makefile.

It does not intentionally delete the remote R2 replica.

After deployment:

```bash
make swarm-logs
```

Verify functional recovery rather than matching exact historical log strings.

## Full Local Swarm Test

> **Local test only:** this target purges local Swarm deployment state and may force the local Docker Engine to leave the Swarm. Do not run it on a production multi-node manager.

Run:

```bash
make swarm-full-test
```

It uses the current `swarm-local-purge` and `swarm-local-deploy` targets to exercise a complete local rebuild.

Consult those targets before using them on any machine with meaningful Swarm state.

## Status and Logs

Show nodes, services, and gateway tasks:

```bash
make swarm-status
```

Follow gateway service logs:

```bash
make swarm-logs
```

Stop the stack:

```bash
make swarm-stop
```

The log target treats the normal interactive Ctrl-C termination path according to the current Makefile implementation.

# Physical Multi-Node Cluster

Host-level node commands are defined in:

```text
cluster/node/Makefile.mk
```

and included by the root Makefile.

The intended architecture uses a private network for Swarm control/data traffic and a separately managed uplink identity for external gateway traffic.

## Private Swarm Network

The private-network variables and defaults are authoritative in:

```text
cluster/node/Makefile.mk
```

Configure the node's private NetworkManager connection with:

```bash
make node-private-network \
  PRIVATE_CONNECTION="<connection-name>" \
  NODE_ADDRESS="<node-address>/<prefix>"
```

The target calls:

```text
scripts/cluster/configure-private-network.sh
```

Each physical node requires a unique private address.

## Initialize the First Manager

Initialize the first Swarm manager on its private address:

```bash
make swarm-init-private \
  NODE_ADDRESS="<node-address>/<prefix>"
```

The target binds the relevant Swarm addresses to the node's private address according to `cluster/node/Makefile.mk`.

Retrieve the manager join token:

```bash
make swarm-manager-token
```

Treat the join token as a cluster credential.

## Join Additional Managers

Join another manager:

```bash
make swarm-join-manager \
  NODE_ADDRESS="<joining-node-address>/<prefix>" \
  MANAGER_IP="<existing-manager-private-ip>" \
  MANAGER_TOKEN="<manager-token>"
```

Where supported by the current Makefile, a token file may be used instead of placing the token directly on the command line.

The joining node must not already be in an incompatible Swarm state.

## Provision the Local Data Volume on Every Eligible Node

Because the external Docker volume is node-local, every node that may host the gateway task needs the configured volume.

Run on each eligible node:

```bash
make swarm-volume
```

If Swarm places the task on a node where the required external volume does not exist, the service cannot mount it.

An existing but empty volume can use the Litestream recovery path if the remote replica is available.

## Shared Uplink Identity

The shared uplink variables are defined in:

```text
cluster/node/Makefile.mk
```

Configure the uplink:

```bash
make swarm-uplink-configure \
  UPLINK_CONNECTION="<connection-name>" \
  SHARED_MAC="<shared-mac>"
```

The target delegates to:

```text
scripts/cluster/configure-uplink.sh
```

The shared uplink identity is host infrastructure, not part of the container image.

## Uplink Controller

Install the uplink controller:

```bash
make swarm-uplink-install \
  UPLINK_CONNECTION="<connection-name>" \
  SHARED_MAC="<shared-mac>" \
  PEER_MANAGER_IPS="<space-separated-peer-private-ips>"
```

The exact optional variables and defaults are defined in `cluster/node/Makefile.mk`.

The implementation uses:

```text
scripts/cluster/install-uplink-controller.sh
scripts/cluster/fbs-swarm-uplink.sh
services/linux/fbs-swarm-uplink.service
```

Status:

```bash
make swarm-uplink-status
```

Uninstall:

```bash
make swarm-uplink-uninstall
```

Use `cluster/node/README.md` for node-specific operational detail.

# Recovery and Failure Behavior

The deployment separates three kinds of state:

```text
Swarm control state
node-local runtime state
gateway persistent application state
```

### Swarm control state

Includes:

- service definition
- service placement
- external-secret references
- manager membership

### Node-local runtime state

Includes:

- external Docker volume
- cached image layers
- NetworkManager profiles
- uplink-controller installation

### Gateway persistent application state

The gateway database path is defined in `litestream.yml` and resides on the mounted `/data` storage.

Litestream replicates that database according to `litestream.yml`.

Expected recovery principles:

| Failure | Expected response |
| --- | --- |
| Gateway process exits | Swarm restart policy handles the task according to `stack.yml` |
| Container task fails | Swarm recreates/reschedules according to `stack.yml` |
| Active node fails | Swarm may schedule the task on another eligible node |
| Replacement node has usable local DB | Gateway can use local state |
| Replacement node has empty data volume | Litestream can apply configured restore behavior |
| Replacement node lacks required external volume | Provision the volume on that node |
| Remote object storage unavailable but local DB exists | Local state may remain usable; remote persistence/recovery is degraded |
| Local DB absent and remote replica unavailable | Automatic database recovery cannot complete |
| Required Docker Secret missing | Task startup fails until secret configuration is corrected |
| Published image unavailable | Swarm cannot pull/start that image |

This project intentionally uses local SQLite plus asynchronous object-storage recovery rather than a shared active SQLite file over NFS/SMB.

# Makefile Reference

The Makefile is the operational interface for normal development and deployment.

## Core Variables

Rather than copying default values into this README, this section lists **variable names and responsibilities**.

### Gateway dependency

```make
GATEWAY_VERSION
```

Pinned upstream gateway release consumed by the Dockerfile.

### Cluster build metadata

```make
CLUSTER_VERSION
COMMIT
DATE
```

Image/source metadata used by local and publication builds.

### Local container

```make
CONTAINER_IMAGE
CONTAINER_TAG
CONTAINER_PLATFORM
```

### Docker Hub

```make
DOCKERHUB_NAMESPACE
DOCKERHUB_IMAGE
DOCKERHUB_TAG
DOCKERHUB_PLATFORMS
BUILDX_BUILDER
```

### Swarm

```make
ENV_FILE
SWARM_STACK
SWARM_SERVICE
SWARM_STACK_FILE
SWARM_VOLUME
SWARM_IMAGE
SWARM_PLATFORM
SWARM_DATA_UID
SWARM_DATA_GID
```

### Swarm secret identifiers

```make
R2_ACCESS_SECRET
R2_SECRET_SECRET
SERVER_CA_SECRET
GATEWAY_CLIENT_CERT_SECRET
GATEWAY_CLIENT_KEY_SECRET
```

### Runtime TLS sources

```make
TLS_SERVER_CA_SOURCE
TLS_CLIENT_CERT_SOURCE
TLS_CLIENT_KEY_SOURCE
```

### Physical node networking

See `cluster/node/Makefile.mk` for the current complete set. It includes variables for:

- private connection/address
- manager address/token
- uplink connection
- shared MAC
- peer managers
- controller polling
- service name

## Development and Validation Targets

| Target | Purpose |
| --- | --- |
| `make fmt` | Format Go source |
| `make tidy` | Tidy Go module state |
| `make test` | Run normal Go tests |
| `make fmt-check` | Verify Go formatting |
| `make tidy-check` | Verify module files are already tidy |
| `make vet` | Run `go vet` |
| `make staticcheck` | Run pinned Staticcheck |
| `make test-race` | Run Go tests under race detector |
| `make scripts-check` | Bash syntax validation |
| `make shellcheck` | ShellCheck cluster scripts |
| `make build` | Build supported `swarm-entrypoint` binaries |
| `make build-check` | Build/inspect entrypoint architectures |
| `make verify` | Run the complete repository validation gate |
| `make clean` | Remove generated build output |

## Container Targets

| Target | Purpose |
| --- | --- |
| `make container-build` | Build the configured local development image |
| `make container-smoke` | Integration-test gateway + entrypoint + Litestream |
| `make container-run` | Run the configured local container path |
| `make container-builder` | Create/select the Buildx builder |
| `make container-login` | Interactive Docker Hub login |
| `make container-publish` | Build/push configured multi-platform release image |
| `make container-inspect` | Inspect configured published manifest |

## Swarm Targets

| Target | Purpose |
| --- | --- |
| `make swarm-preflight` | Validate deployment prerequisites |
| `make swarm-image` | Pull configured published Swarm image |
| `make swarm-init` | Initialize Swarm if needed |
| `make swarm-volume` | Create/prepare external data volume |
| `make swarm-volume-reset` | Destructively recreate local data volume |
| `make swarm-r2-secrets` | Create R2 Docker Secrets |
| `make swarm-cert-secrets` | Create runtime TLS Docker Secrets |
| `make swarm-secrets` | Create all required secrets |
| `make swarm-secrets-remove` | Remove cluster secrets when safe |
| `make swarm-secrets-recreate` | Recreate cluster secrets |
| `make swarm-stop` | Remove stack and wait for service removal |
| `make swarm-deploy` | Deploy using `SWARM_IMAGE` |
| `make swarm-wait` | Wait for configured service readiness condition |
| `make swarm-local-deploy` | Normal local/prepared-manager deployment |
| `make swarm-reset-deploy` | Destructive local-volume recovery test |
| `make swarm-status` | Show Swarm deployment status |
| `make swarm-logs` | Follow gateway service logs |
| `make swarm-local-purge` | Purge local cluster test state |
| `make swarm-full-test` | Purge and rebuild the local test deployment |

## Physical Node Targets

The current complete target set is authoritative in `cluster/node/Makefile.mk`.

The node module provides targets for:

- private-network configuration
- private-address Swarm initialization
- manager token retrieval
- manager join
- uplink configuration
- uplink controller installation
- uplink controller status
- uplink controller removal

# Continuous Integration

CI behavior is authoritative in:

```text
.github/workflows/ci.yml
```

The workflow runs on the events currently declared there and validates both source and container behavior.

The intended CI coverage includes:

- repository checkout with pinned GitHub Actions
- Go setup from `go.mod`
- `make verify`
- local container smoke testing
- cross-architecture container build validation
- image architecture validation
- Swarm stack syntax/interpolation validation

Do not copy the exact action versions, architecture list, or step wording from this README when modifying CI; edit and review the workflow itself.

Dependabot behavior is authoritative in:

```text
.github/dependabot.yml
```

# Release Workflow

Official releases are created by:

```text
.github/workflows/release.yml
```

The workflow is manually triggered with a cluster release version.

The workflow's exact permissions, job ordering, credentials, validation steps, image aliases, and verification behavior are authoritative in that file.

## Release Environment

The protected GitHub environment is named:

```text
release
```

The workflow currently expects protected credentials/variables for:

- Docker Hub authentication
- GPG release signing
- GPG fingerprint/committer identity

GHCR authentication uses the workflow-provided GitHub token together with the publish job's `packages: write` permission. It does not require an additional GHCR secret in the protected environment.

The exact secret and variable names are authoritative in `.github/workflows/release.yml`.

Do not maintain a second required-secret list in operational automation based only on this README.

## Release Stages

Conceptually, the release workflow has three trust stages.

### 1. Preflight

The workflow validates:

- release runs from the intended branch
- requested release version syntax
- requested tag does not already exist
- pinned upstream gateway release is valid/available according to the workflow's current checks

### 2. Source and container validation

The validation job exercises:

- repository validation
- container smoke test
- secondary-architecture build
- image architecture inspection
- Swarm stack validation

The exact architecture/platform list comes from workflow/Makefile source.

### 3. Protected publication

The protected publish job:

- derives the Docker tag from the requested Git release version
- authenticates to Docker Hub
- authenticates to GHCR using the workflow-provided GitHub token
- calls the Makefile publication target once to build and publish the multi-platform image to Docker Hub
- copies that published OCI image index to GHCR without rebuilding it
- applies the stable `latest` alias to both registries when the workflow's stable-version rule is satisfied
- verifies the expected image platforms in both registries
- records both registry image-index digests
- fails if the Docker Hub and GHCR digests differ
- writes both registry references and digests into release metadata
- imports the protected GPG key
- creates/verifies/pushes a signed Git tag
- creates the GitHub Release
- verifies the immutable release and release metadata asset

## Release Metadata

The GitHub release contains:

```text
release-metadata.txt
```

The current workflow writes fields representing:

```text
cluster version
upstream gateway version
Docker Hub image reference
Docker Hub OCI image-index digest
GHCR image reference
GHCR OCI image-index digest
source commit
published platforms
```

A conceptual example is:

```text
cluster_version=<git-release-version>
gateway_version=<pinned-gateway-version>
image=<docker-hub-repository>:<docker-tag>
digest=sha256:<docker-hub-index-digest>
ghcr_image=ghcr.io/<repository-owner>/<repository-name>:<docker-tag>
ghcr_digest=sha256:<ghcr-index-digest>
source_commit=<git-commit>
platforms=<published-platform-list>
```

The existing `image=` and `digest=` fields continue to describe Docker Hub. The additional `ghcr_image=` and `ghcr_digest=` fields describe the GitHub Container Registry mirror.

The release workflow requires both digest values to be equal before it proceeds.

No concrete release version belongs in this README.

The metadata file is the release-specific record of exactly what was published.

## Immutable Releases

GitHub release immutability must be enabled for the repository when the workflow relies on:

```text
gh release verify
```

An old release created before immutability was enabled does not become attested retroactively merely because the repository setting changes later.

If verification repeatedly reports that no attestation exists for an already-created release, verify whether that release was created under the required immutability settings.

## Release Verification

Given a release tag:

```bash
RELEASE_TAG="<release-tag>"
```

Verify the signed Git tag:

```bash
git fetch origin --tags
git tag -v "${RELEASE_TAG}"
```

Verify the immutable GitHub Release:

```bash
gh release verify "${RELEASE_TAG}" \
  --repo williamveith/fbs-interlock-gateway-cluster
```

Download and verify release metadata:

```bash
gh release download "${RELEASE_TAG}" \
  --repo williamveith/fbs-interlock-gateway-cluster \
  --pattern release-metadata.txt
```

```bash
gh release verify-asset \
  "${RELEASE_TAG}" \
  release-metadata.txt \
  --repo williamveith/fbs-interlock-gateway-cluster
```

For container verification, use the registry references and digests recorded in `release-metadata.txt` rather than assuming a tag from this README.

The two recorded registry digests should be identical:

```text
digest == ghcr_digest
```

Inspect either published reference with:

```bash
docker buildx imagetools inspect "<image-reference>"
```

The normal output should show the configured runnable platform manifests plus any OCI attestation manifests generated by the Buildx provenance/SBOM settings.

# Updating the Upstream Gateway

The upstream gateway is an explicit versioned dependency.

When a new gateway release is ready for cluster integration:

1. review the upstream release
2. change `GATEWAY_VERSION` in the Makefile to the desired upstream release tag
3. run the repository validation gate
4. run the container smoke test
5. confirm the smoke test reports the newly configured gateway version
6. test Swarm behavior as appropriate
7. release a new cluster version rather than silently rebuilding an existing published cluster release with different gateway contents

Commands:

```bash
make verify
make container-smoke
```

The important invariant is:

```text
one published cluster release
    ->
one recorded upstream gateway dependency
```

The exact version pair is recorded by the release metadata, not by this README.

# Releasing a New Cluster Version

The deployment default is controlled by:

```make
DOCKERHUB_TAG
```

The release workflow receives a Git-style release tag and derives its Docker tag according to the workflow's current logic.

Before release:

1. update `GATEWAY_VERSION` if the cluster is intentionally moving to a new gateway release
2. update `DOCKERHUB_TAG` to the Docker tag intended for the new cluster release
3. run:

```bash
make verify
make container-smoke
```

4. commit and push the changes to the release branch expected by the workflow
5. trigger the workflow with the desired release tag

Example using placeholders:

```bash
gh workflow run release.yml \
  --ref main \
  -f version="v<MAJOR>.<MINOR>.<PATCH>"
```

After a successful release, the same OCI image index is available through both Docker Hub and GHCR. The release-specific registry references are recorded in `release-metadata.txt`.

Swarm deployment still uses the image selected by the Makefile:

```bash
make swarm-local-deploy
```

For a destructive recovery validation:

```bash
make swarm-reset-deploy
make swarm-logs
```

# Branch and Pull-Request Workflow

Start from current `main`:

```bash
git switch main
git pull --ff-only origin main
```

Create a branch:

```bash
git switch -c feature/<description>
```

Validate before committing:

```bash
make fmt
make verify
make container-smoke
git status
git diff
```

Commit and push:

```bash
git add -A
git commit -S -m "Describe the change"
git push --set-upstream origin feature/<description>
```

Open a pull request:

```bash
gh pr create \
  --base main \
  --head feature/<description> \
  --fill
```

After merging:

```bash
git switch main
git pull --ff-only origin main
git fetch --prune
git branch -d feature/<description>
```

# Security Model

The cluster repository adds deployment-layer controls around the security implemented by the upstream gateway.

Important boundaries:

- hardware interlocks and fail-safe circuitry remain authoritative
- the cluster is designed around one active gateway task
- the container runs as a non-root numeric identity defined by source
- the final image uses a scratch-style Litestream runtime
- R2 credentials are supplied through Docker Secrets in Swarm deployments
- gateway runtime TLS material is supplied through Docker Secrets
- secret ownership/mode is controlled by the stack file
- the upstream gateway version is explicitly pinned
- the Dockerfile validates the downloaded gateway binary against the selected release checksum
- the protected release workflow validates the pinned upstream release before cluster publication
- official images include the configured BuildKit provenance/SBOM attestations
- the protected release workflow mirrors the same OCI image index from Docker Hub to GHCR instead of rebuilding it
- publication fails if Docker Hub and GHCR report different OCI index digests
- GHCR publication uses the workflow GitHub token with `packages: write` rather than a separate long-lived registry credential
- Git release tags are GPG-signed
- GitHub Releases are verified after publication
- release metadata records the exact Docker manifest digest
- SQLite remains node-local rather than being actively shared over NFS/SMB
- R2 provides replicated recovery rather than shared-file locking

Network exposure remains an operational responsibility.

The **current** published ports are defined in:

```text
cluster/swarm/stack.yml
```

The current gateway bind/runtime command is defined in:

```text
litestream.yml
```

Review both before implementing firewall, routing, or remote-management rules.

Application-level request validation, Admin protections, Shelly authentication, and configuration security belong to the upstream gateway project.

# Troubleshooting

## `make container-smoke` reports the wrong gateway version

Check the live dependency:

```bash
grep '^GATEWAY_VERSION' Makefile
```

Then rebuild:

```bash
make container-smoke
```

The smoke test compares the gateway's runtime `-version` output against the configured `GATEWAY_VERSION`.

## Container build fails during checksum verification

The checksum verification command is defined in `Dockerfile`.

If the base image/tool implementation changes, use the syntax supported by that build-stage environment.

Do not preserve a historical checksum command in this README as authoritative; inspect the Dockerfile and the failing tool's usage output.

## Swarm service does not become ready

Run:

```bash
make swarm-status
```

and:

```bash
make swarm-logs
```

Use the Makefile's status and log targets as the normal diagnostic interface. If deeper Docker inspection is required, resolve the current service name from the Makefile rather than copying a historical service name from documentation.

Typical classes of failure include:

- required external volume missing on the scheduled node
- required Docker Secret missing
- unreadable secret
- image pull failure
- R2 connectivity/credential failure
- no usable local database and no recoverable remote state
- upstream gateway configuration/TLS validation failure
- published port conflict
- incompatible Swarm/node state

## R2 restore does not occur

Check the restore configuration in:

```bash
cat litestream.yml
```

Restore behavior depends on the current local database state and the Litestream configuration.

For a deliberate recovery test:

```bash
make swarm-reset-deploy
```

Then:

```bash
make swarm-logs
```

## Secret recreation fails

The Makefile protects secret removal while the service is still using those secrets.

Use:

```bash
make swarm-stop
make swarm-secrets-recreate
```

## External volume reset fails

The Makefile protects destructive volume reset while the service still exists.

Use:

```bash
make swarm-stop
make swarm-volume-reset
```

## Physical node cannot join Swarm

Check:

- Docker Swarm state on the joining node
- the private node address supplied to the target
- reachability of the existing manager's private address
- manager join token
- Swarm manager/control-plane firewalling
- active NetworkManager profile

Use `cluster/node/Makefile.mk` and `cluster/node/README.md` for the exact current parameters.

## Published image is not the expected release

Check:

```bash
grep -E '^(DOCKERHUB_IMAGE|DOCKERHUB_TAG|SWARM_IMAGE)' Makefile
```

Then:

```bash
make swarm-image
make swarm-deploy
```

The stack consumes `SWARM_IMAGE`; it should not maintain a separate release number.

## `gh release verify` reports no attestation

Check repository release-immutability settings and whether the affected release was originally created under those settings.

Also inspect the current verification logic in:

```text
.github/workflows/release.yml
```

Do not assume increasing retry time will fix a release for which no attestation can be created.

# Repository Safety

Generated/local state should remain separate from source.

Examples of local or generated material include:

```text
build output
environment files
local Docker state
local Swarm state
gateway SQLite state
production gateway configuration
R2 credentials
private release-signing credentials
```

The actual ignore policy is authoritative in:

```text
.gitignore
.dockerignore
```

The Docker build context is intentionally constrained so image construction receives only the local repository files the Dockerfile needs.

Review the current context policy with:

```bash
cat .dockerignore
```

Review source-control exclusions with:

```bash
cat .gitignore
```

Release-signing keys and Docker Hub credentials belong in the protected GitHub release environment, not in source control.

# License

See [`LICENSE.md`](LICENSE.md) for the terms governing this project.
