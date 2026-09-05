---
title: "FBS Interlock Gateway"
subtitle: "macOS Installation and Operations Guide"
author: "William Veith"
date: "2026-09-04"
lang: en-US
---

> **Purpose**
>
> This guide covers building, transferring, installing, validating, operating, updating, and uninstalling `fbs-interlock-gateway-cluster` on a dedicated macOS gateway computer.

> **Security boundary**
>
> The gateway controls software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. The installer restricts the FBS listener range with a managed macOS Packet Filter (`pf`) anchor, but the Admin UI must remain bound to loopback unless remote access is intentionally secured.

## Table of Contents

- [Deployment Overview](#deployment-overview)
- [Supported Mac Architectures](#supported-mac-architectures)
- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Prepare Gateway TLS Material](#prepare-gateway-tls-material)
- [Build the Deployment Assets](#build-the-deployment-assets)
- [Transfer the Deployment Directory](#transfer-the-deployment-directory)
- [Choose an Installation Mode](#choose-an-installation-mode)
- [Install the Gateway](#install-the-gateway)
- [What the Installer Does](#what-the-installer-does)
- [Installed Layout](#installed-layout)
- [Verify the Installation](#verify-the-installation)
- [Verify Tool Communication](#verify-tool-communication)
- [View the Admin Panel](#view-the-admin-panel)
- [Automatic Updates and Log Maintenance](#automatic-updates-and-log-maintenance)
- [View Gateway Logs](#view-gateway-logs)
- [Edit the Configuration](#edit-the-configuration)
- [Restart the Gateway](#restart-the-gateway)
- [Firewall and Packet Filter Behavior](#firewall-and-packet-filter-behavior)
- [Gatekeeper and Quarantine](#gatekeeper-and-quarantine)
- [Troubleshooting](#troubleshooting)
- [Uninstall the Gateway](#uninstall-the-gateway)
- [Command Reference](#command-reference)

<div class="page-break"></div>

# Deployment Overview

The macOS deployment uses two system-wide `launchd` jobs:

1. The gateway LaunchDaemon starts before a user signs in and keeps the gateway process running.
2. The production update LaunchDaemon checks the latest published release once per hour and performs binary and log maintenance when required.

```text
FBS server
    -> managed macOS pf source restriction
    -> macOS gateway listener port
    -> fbs-interlock-gateway-cluster
    -> Shelly RPC over HTTP or HTTPS
    -> tool interlock circuit
```

The installed Admin UI remains local by default:

```text
http://127.0.0.1:18090
```

Normal FBS `/status`, `/on`, and `/off` requests update the Admin status cache for the affected tool. The browser reads that in-memory cache without generating recurring Shelly requests. The **Refresh Status** action performs an explicit fleet-wide verification.

## Deployment sequence

1. Identify the Mac architecture.
2. Generate the required gateway runtime TLS files.
3. Build the matching deployment directory.
4. Copy the complete deployment directory to the gateway Mac.
5. Choose production or development installation mode.
6. Run the installer with administrator privileges.
7. Verify the gateway LaunchDaemon, update LaunchDaemon, Admin API, TLS access, `pf` rules, tool ports, and logs.

# Supported Mac Architectures

Determine the gateway Mac architecture:

```bash
uname -m
```

| Output | Mac type | Build target | Deployment directory |
| --- | --- | --- | --- |
| `arm64` | Apple Silicon | `make build-darwin-arm64` | `build/darwin/arm64/` |
| `x86_64` | Intel | `make build-darwin-amd64` | `build/darwin/amd64/` |

> **Important**
>
> Use the deployment directory that matches the gateway Mac. The installer validates the Mach-O architecture before replacing any installed files and exits when the binary does not match the machine.

# Pre-Deployment Checklist

Before building or installing, confirm the following:

- The repository is on the intended commit or release.
- `make verify` completes successfully.
- The packaged `config.yaml` contains the intended seed configuration for a fresh installation or legacy rollback. After first startup, `gateway.sqlite3` becomes authoritative.
- `make ca` and `make gateway-cert` have populated the required certificate material under `pki/ca/` and `pki/gateway/`.
- The selected build target matches the gateway Mac architecture.
- The configured FBS listener ports do not conflict with other services.
- `FBS_SOURCE_IP` and `FBS_PORT_RANGE` in the Makefile are correct before building.
- The Admin address remains `127.0.0.1:18090` unless a reviewed remote-access design is being used.
- The gateway Mac can resolve every configured Shelly hostname.
- The gateway Mac can reach GitHub Releases when managed automatic updates are required.
- The deployment directory will be transferred and stored as a complete unit.

Current generated firewall defaults are:

```text
Authorized FBS source: 146.6.76.61
Gateway listener range: TCP 8081:8981
```

Review the generated Packet Filter file before production installation:

```text
com.williamveith.fbs-interlock-gateway-cluster.pf
```

# Prepare Gateway TLS Material

All macOS deployment builds require the gateway runtime TLS files. The canonical certificate material remains in the PKI directories; the Makefile copies only the runtime files required by the gateway into the deployment package.

Generate the certificate authorities and gateway client identity on the controlled development machine:

```bash
make ca
make gateway-cert
```

The required source files are:

```text
pki/
├── ca/
│   └── server-ca.crt
└── gateway/
    ├── gateway-client.crt
    └── gateway-client.key
```

The macOS build copies these files directly into the architecture-specific deployment directory:

```text
build/darwin/<architecture>/tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Do not copy the complete `pki/` directory into the deployment package.

> **Private-key handling**
>
> Never place CA private keys, certificate requests, or Shelly server private keys on the gateway Mac. Only the gateway runtime server CA certificate, gateway client certificate, and gateway client private key are packaged.

The build fails before producing a macOS deployment package when any required runtime TLS file is missing.

# Build the Deployment Assets

On the development Mac, start from a clean build directory.

## Apple Silicon

```bash
make clean
make build-darwin-arm64
```

Generated directory:

```text
build/darwin/arm64/
```

## Intel

```bash
make clean
make build-darwin-amd64
```

Generated directory:

```text
build/darwin/amd64/
```

The selected deployment directory contains:

```text
build/darwin/<architecture>/
├── fbs-interlock-gateway-cluster
├── config.yaml
├── install.sh
├── install-dev.sh
├── start.sh
├── uninstall.sh
├── update.sh
├── com.williamveith.fbs-interlock-gateway-cluster.plist
├── com.williamveith.fbs-interlock-gateway-cluster-update.plist
├── com.williamveith.fbs-interlock-gateway-cluster.pf
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── macOS Installation and Operations Guide.pdf
```

The staged TLS modes are:

| File | Staged mode |
| --- | --- |
| `server-ca.crt` | `0644` |
| `gateway-client.crt` | `0644` |
| `gateway-client.key` | `0600` |

> **Do not edit generated deployment files directly.**
>
> Update the source templates, deployment guide, or Makefile variables and rebuild instead.

# Transfer the Deployment Directory

## Copy to a USB drive

Copy the complete architecture-specific deployment directory to a USB flash drive.

Do not copy only the executable. The complete directory contains:

- The application executable
- Production and development installers
- The startup wrapper
- The gateway and update LaunchDaemon property lists
- The checksum-aware updater
- The Packet Filter anchor
- The uninstaller
- The configuration file
- The gateway runtime TLS files
- The deployment guide PDF

## Copy to the gateway Mac

Insert the USB drive into the gateway Mac and copy the complete directory into the current user's `Downloads` directory.

Apple Silicon example:

```text
~/Downloads/darwin/arm64/
```

Intel example:

```text
~/Downloads/darwin/amd64/
```

Confirm the complete package is present:

```bash
find ~/Downloads/darwin/arm64 -maxdepth 2 -print
```

<div class="page-break"></div>

# Choose an Installation Mode

The deployment package supports two installation modes.

## Production installation

Use the normal installer for a managed production gateway:

```bash
sudo ./install.sh
```

Production mode:

- Installs the local gateway binary
- Installs and enables the hourly update LaunchDaemon
- Installs `update.sh`
- Performs checksum-aware updates from GitHub Releases
- Rotates gateway logs when they reach the configured size limit
- Restores managed updates after a previous development installation

## Development installation

Use the development installer when testing an unpublished local build:

```bash
sudo ./install-dev.sh
```

This is equivalent to:

```bash
sudo ./install.sh --development
```

Development mode:

- Installs the local gateway binary and all production security controls
- Preserves the authoritative SQLite configuration, YAML rollback/compatibility state, and installed TLS files
- Stops and disables the update LaunchDaemon
- Removes the installed updater script and update plist
- Prevents the development binary from being replaced by the latest published release

Run the normal production installer later to restore managed automatic updates.

# Install the Gateway

Open Terminal and move to the copied deployment directory.

Apple Silicon example:

```bash
cd ~/Downloads/darwin/arm64
```

Intel example:

```bash
cd ~/Downloads/darwin/amd64
```

Make the deployment files executable:

```bash
chmod +x \
  install.sh \
  install-dev.sh \
  start.sh \
  uninstall.sh \
  update.sh \
  fbs-interlock-gateway-cluster
```

Run the selected installer.

Production:

```bash
sudo ./install.sh
```

Development:

```bash
sudo ./install-dev.sh
```

Enter the administrator password when prompted.

The installer validates the packaged binary architecture and executes its `-version` command before stopping the existing gateway.

> **Reinstallation behavior**
>
> Reinstallation preserves the authoritative `gateway.sqlite3` database, the YAML rollback mirror, and installed TLS files. It corrects ownership and modes without replacing persistent configuration.

# What the Installer Does

The installer performs the following operations.

## Preflight validation

- Verifies that the installer is running on macOS
- Elevates through `sudo` when required
- Requires the binary, seed config, startup wrapper, gateway plist, Packet Filter anchor, and all three TLS files
- Requires the updater and update plist in production mode
- Validates generated property lists with `plutil`
- Confirms the binary is a Mach-O executable for the current architecture
- Executes the binary's `-version` command

## Service account

- Creates the hidden `_fbs-gateway` group and service account when needed
- Uses a non-login shell and `/var/empty` home directory
- Preserves the account during uninstall so a later reinstall keeps stable ownership

## Application and configuration

- Installs the binary and startup wrapper under `/usr/local/libexec/fbs-interlock-gateway-cluster/`
- Runs the gateway with `/Library/Application Support/fbs-interlock-gateway-cluster` as its working directory
- Starts the gateway with explicit `-config` and `-db` paths
- Installs or preserves `config.yaml` with service-account access and mode `0640`
- Uses `/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3` as the authoritative database path
- On first successful startup, imports the legacy YAML if the database is uninitialized
- Leaves the original human-authored YAML untouched during the first import
- Treats `gateway.sqlite3` as authoritative after initialization
- Later Admin saves generate a YAML compatibility mirror and preserve the previous YAML as `config.yaml.bak` when possible
- Removes the executable quarantine attribute when present

## Gateway TLS files

- Creates `/Library/Application Support/fbs-interlock-gateway-cluster/tls/`
- Installs new runtime TLS files with `root:_fbs-gateway` ownership and mode `0640`
- Preserves existing installed TLS files during reinstallation
- Verifies that `_fbs-gateway` can read the YAML rollback mirror and every TLS file

## SQLite state validation

- Stops the existing gateway before taking the installation rollback copy of `gateway.sqlite3`
- Backs up the database when present
- Verifies that `_fbs-gateway` can create and remove files in `/Library/Application Support/fbs-interlock-gateway-cluster/`
- Starts the gateway and waits for the Admin API
- Verifies that `gateway.sqlite3` exists, is non-empty, and is readable and writable by `_fbs-gateway`
- Removes transient SQLite sidecar files before restoring a database during installation rollback

## LaunchDaemons

- Installs the main gateway LaunchDaemon
- Explicitly enables the main LaunchDaemon before bootstrapping it into `launchd`
- Uses a ten-second restart throttle
- Uses a 30-second exit timeout and restrictive process umask
- Starts the gateway immediately and waits for the Admin API health check
- Installs the update LaunchDaemon only in production mode
- Schedules production updates at minute 17 of every hour
- Restores prior enabled/loaded state during installation rollback when possible

## Firewall controls

- Adds the executable to the macOS Application Firewall allow list
- Installs a named `pf` anchor
- Adds a managed block to `/etc/pf.conf`
- Validates the complete generated `pf` configuration before replacing `/etc/pf.conf`
- Reloads Packet Filter and enables it when necessary
- Allows loopback access to the listener range
- Allows the configured FBS source IP to the listener range
- Blocks every other inbound TCP connection to that range

## Logs

Creates:

```text
/Library/Logs/fbs-interlock-gateway-cluster/
├── gateway.log
├── gateway-error.log
├── update.log
└── update-error.log
```

Gateway logs are owned by the gateway service account. Update logs are owned by `root:wheel`.

## Rollback and health validation

Before replacing an existing installation, the installer backs up the currently installed binary, wrappers, plists, Packet Filter anchor, `/etc/pf.conf`, and authoritative SQLite database when present.

If Packet Filter validation, LaunchDaemon loading, Admin API health validation, or SQLite validation fails, the installer restores the previous executable, service/network files, and database and attempts to restart the previous installation.

The preserved YAML rollback mirror and installed TLS files are not replaced during normal installation.

# Installed Layout

## Application directory

```text
/usr/local/libexec/fbs-interlock-gateway-cluster/
├── fbs-interlock-gateway-cluster
├── start.sh
└── update.sh                 # production mode only
```

## Authoritative configuration, rollback mirror, and TLS

```text
/Library/Application Support/fbs-interlock-gateway-cluster/
├── gateway.sqlite3           # authoritative SQLite configuration
├── config.yaml               # first-run seed; later generated rollback mirror
├── config.yaml.bak           # previous YAML mirror when available
└── tls/
    ├── server-ca.crt
    ├── gateway-client.crt
    └── gateway-client.key
```

After `gateway.sqlite3` is initialized, manual edits to `config.yaml` are ignored by normal gateway startup. Use the Admin UI or the documented export/import workflow to change the authoritative configuration.

## LaunchDaemons

```text
/Library/LaunchDaemons/
├── com.williamveith.fbs-interlock-gateway-cluster.plist
└── com.williamveith.fbs-interlock-gateway-cluster-update.plist
```

The update plist is absent after a development installation.

## Packet Filter

```text
/etc/pf.anchors/com.williamveith.fbs-interlock-gateway-cluster
/etc/pf.conf
```

The installer places a clearly marked managed anchor block in `/etc/pf.conf` rather than replacing unrelated Packet Filter rules.

## Logs

```text
/Library/Logs/fbs-interlock-gateway-cluster/
├── gateway.log
├── gateway-error.log
├── update.log
└── update-error.log
```

<div class="page-break"></div>

# Verify the Installation

## Check the installed version

```bash
sudo "/usr/local/libexec/fbs-interlock-gateway-cluster/fbs-interlock-gateway-cluster" \
  -version
```

## Check the gateway LaunchDaemon

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-cluster
```

Look for a running state and process identifier:

```text
state = running
pid = <process-id>
```

## Check the update LaunchDaemon

Production installation:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-cluster-update
```

A development installation should not have this job loaded.

## Check the Admin API

Read the current in-memory status snapshot:

```bash
curl -i http://127.0.0.1:18090/api/status
```

A successful response should include:

```text
HTTP/1.1 200 OK
X-Status-Refresh-In-Progress: false
```

Start an explicit fleet refresh:

```bash
curl -i \
  "http://127.0.0.1:18090/api/status?refresh=1"
```

During the scan, the response header reports:

```text
X-Status-Refresh-In-Progress: true
```

## Confirm listening ports

Admin port:

```bash
sudo lsof -nP -iTCP:18090 -sTCP:LISTEN
```

Example FBS listener port:

```bash
sudo lsof -nP -iTCP:8081 -sTCP:LISTEN
```

## Verify configuration database, rollback mirror, and TLS permissions

Confirm the authoritative database exists and is non-empty:

```bash
sudo test -s \
  "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3"
```

Confirm `_fbs-gateway` can read and write it:

```bash
sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3"

sudo -u _fbs-gateway test -w \
  "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3"
```

Confirm the service account can read the YAML rollback mirror and runtime TLS files:

```bash
sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway-cluster/config.yaml"

sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway-cluster/tls/server-ca.crt"

sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway-cluster/tls/gateway-client.crt"

sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway-cluster/tls/gateway-client.key"
```

Each command should exit silently with status `0`.

## Verify Packet Filter

Display the installed anchor rules:

```bash
sudo pfctl \
  -a com.williamveith.fbs-interlock-gateway-cluster \
  -sr
```

Check Packet Filter status:

```bash
sudo pfctl -s info
```

Confirm the managed block exists:

```bash
sudo grep -A 4 -B 1 \
  "BEGIN fbs-interlock-gateway-cluster managed anchor" \
  /etc/pf.conf
```

## Confirm Application Firewall registration

```bash
sudo /usr/libexec/ApplicationFirewall/socketfilterfw \
  --listapps | grep -A 3 fbs-interlock-gateway-cluster
```

# Verify Tool Communication

Replace `8081` with the configured listener port for the tool being tested.

Read the tool status:

```bash
curl http://127.0.0.1:8081/status
```

Turn the tool output on:

```bash
curl http://127.0.0.1:8081/on
```

Turn the tool output off:

```bash
curl http://127.0.0.1:8081/off
```

Expected FBS-compatible responses:

```json
{"Success":1,"State":1}
```

```json
{"Success":1,"State":0}
```

> **Operational caution**
>
> `/on` and `/off` operate the configured interlock. Perform command testing only when changing the interlock state is authorized and safe.

## Verify HTTPS or mutual TLS directly

```bash
curl \
  --cacert "/Library/Application Support/fbs-interlock-gateway-cluster/tls/server-ca.crt" \
  --cert "/Library/Application Support/fbs-interlock-gateway-cluster/tls/gateway-client.crt" \
  --key "/Library/Application Support/fbs-interlock-gateway-cluster/tls/gateway-client.key" \
  "https://<shelly-ddns-host>/rpc/Switch.GetStatus?id=0"
```

Add Digest Authentication when the Shelly also requires it:

```bash
curl \
  --anyauth \
  -u "admin:<password>" \
  --cacert "/Library/Application Support/fbs-interlock-gateway-cluster/tls/server-ca.crt" \
  --cert "/Library/Application Support/fbs-interlock-gateway-cluster/tls/gateway-client.crt" \
  --key "/Library/Application Support/fbs-interlock-gateway-cluster/tls/gateway-client.key" \
  "https://<shelly-ddns-host>/rpc/Switch.GetStatus?id=0"
```

# View the Admin Panel

On the gateway machine, open the following address in a web browser:

<http://127.0.0.1:18090>

The Admin UI provides:

- Current cached status for every configured tool
- Connected, disconnected, output, protocol, and error information
- Passive updates from normal FBS traffic
- A **Refresh Status** button for explicit fleet verification
- Configuration editing and validation
- Password replacement or clearing without exposing stored passwords
- Gateway restart after a successful configuration save

## Passive status behavior

The browser polls only the gateway's in-memory status cache every three seconds. These cache reads do not contact Shelly devices.

```text
FBS request
    -> Shelly operation
    -> shared status row updated
    -> Admin browser reads cache
```

Use **Refresh Status** when an independent `Switch.GetStatus` check is required.

## Remote Admin access

Keep the Admin UI bound to loopback. For temporary remote access, use an SSH tunnel from an authorized computer:

```bash
ssh -L 18090:127.0.0.1:18090 \
  <mac-user>@<gateway-host>
```

Then open locally:

```text
http://127.0.0.1:18090
```

# Automatic Updates and Log Maintenance

Production installation creates:

```text
system/com.williamveith.fbs-interlock-gateway-cluster-update
```

The update LaunchDaemon runs at minute `17` of every hour and executes:

```text
/usr/local/libexec/fbs-interlock-gateway-cluster/update.sh
```

## Update behavior

The updater:

1. Acquires a lock so concurrent update runs exit safely.
2. Detects Apple Silicon or Intel architecture.
3. Downloads the matching `.sha256` file and detached `.sha256.sig`.
4. Uses the currently installed gateway binary to authenticate the signed checksum and asset name.
5. Computes the installed binary SHA-256 with `shasum -a 256`.
6. Exits without downloading the binary when the authenticated checksum already matches and logs do not need rotation.
7. Downloads the matching `darwin-arm64` or `darwin-amd64` release only when needed.
8. Verifies the downloaded checksum, Mach-O architecture, and version metadata and rejects authenticated downgrades.
9. Creates a timestamped backup of the installed binary.
10. Stops the gateway only when it was loaded before maintenance.
11. Installs and verifies the new binary.
12. Restarts the gateway and waits for the Admin API.
13. Restores the previous binary when the post-update health check fails.

The updater does not intentionally replace `gateway.sqlite3`, the YAML rollback mirror, or installed TLS files. The Admin API health check confirms that the updated process successfully opened its configured SQLite database before the update is accepted.

Installation-time rollback is separate: `install.sh` backs up and restores `gateway.sqlite3` when a new local installation fails validation.

## Log rotation

The updater also checks:

```text
gateway.log
gateway-error.log
```

When either file reaches `10 MiB`, it:

- Stops the gateway when it is loaded
- Compresses the current log with `gzip`
- Shifts existing numbered archives
- Retains up to 30 compressed archives
- Creates a new service-owned log file
- Restarts the gateway and verifies the Admin API

## Run maintenance manually

```bash
sudo "/usr/local/libexec/fbs-interlock-gateway-cluster/update.sh"
```

## Disable managed updates for development

Use the packaged development installer:

```bash
sudo ./install-dev.sh
```

## Restore managed updates

Run the normal production installer:

```bash
sudo ./install.sh
```

# View Gateway Logs

Gateway standard output:

```text
/Library/Logs/fbs-interlock-gateway-cluster/gateway.log
```

Gateway standard error:

```text
/Library/Logs/fbs-interlock-gateway-cluster/gateway-error.log
```

Updater standard output:

```text
/Library/Logs/fbs-interlock-gateway-cluster/update.log
```

Updater standard error:

```text
/Library/Logs/fbs-interlock-gateway-cluster/update-error.log
```

Follow all logs:

```bash
sudo tail -F \
  "/Library/Logs/fbs-interlock-gateway-cluster/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway-cluster/gateway-error.log" \
  "/Library/Logs/fbs-interlock-gateway-cluster/update.log" \
  "/Library/Logs/fbs-interlock-gateway-cluster/update-error.log"
```

Useful gateway markers include:

```text
FBS_IN
FBS_OUT
shelly_status_retry
shelly_reboot_scheduled
shelly_reboot_requested
shelly_reboot_failed
```

Network failures may identify the observed phase:

```text
phase=dns_lookup
phase=tcp_connect
phase=tls_handshake
phase=response_headers
phase=response_body
```

# Edit the Configuration

The authoritative configuration is:

```text
/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3
```

The compatibility/rollback YAML is:

```text
/Library/Application Support/fbs-interlock-gateway-cluster/config.yaml
```

`config.yaml` is used as an import source only while the SQLite database is uninitialized. Once `gateway.sqlite3` contains configuration, normal startup loads SQLite and ignores manual edits to the YAML file.

## Preferred method: Admin UI

```text
http://127.0.0.1:18090
```

The Admin UI validates the complete proposed configuration, preserves stored passwords unless explicitly replaced or cleared, commits the change transactionally to SQLite, writes a generated YAML compatibility mirror when possible, preserves the previous YAML as `config.yaml.bak`, and requests a clean gateway restart.

## Manual method: export, edit, and import

Export the authoritative database:

```bash
sudo "/usr/local/libexec/fbs-interlock-gateway-cluster/fbs-interlock-gateway-cluster" \
  config export \
  -db "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3" \
  -output /tmp/fbs-interlock-gateway-cluster.yaml
```

Edit the exported YAML:

```bash
sudo nano /tmp/fbs-interlock-gateway-cluster.yaml
```

Import the complete edited configuration transactionally and refresh the YAML rollback mirror:

```bash
sudo "/usr/local/libexec/fbs-interlock-gateway-cluster/fbs-interlock-gateway-cluster" \
  config import \
  -db "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3" \
  -input /tmp/fbs-interlock-gateway-cluster.yaml \
  -mirror-config "/Library/Application Support/fbs-interlock-gateway-cluster/config.yaml"
```

Restart the gateway after a successful CLI import:

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway-cluster
```

For review or sharing, create a redacted export:

```bash
sudo "/usr/local/libexec/fbs-interlock-gateway-cluster/fbs-interlock-gateway-cluster" \
  config export \
  -db "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3" \
  -output /tmp/fbs-interlock-gateway-cluster-redacted.yaml \
  -redact-secrets
```

Do not import a redacted export as production configuration; stored passwords are replaced with the literal value `REDACTED`.

> **Do not edit `config.yaml` as the normal configuration workflow**
>
> After SQLite initialization, direct YAML edits do not change the running configuration. Use the Admin UI or `config export` / `config import`.

> **Configuration rule**
>
> `tools[].ip` must contain only the hostname or IP address. Do not include `http://`, `https://`, a path, or a port.

# Restart the Gateway

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway-cluster
```

Verify:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-cluster
```

Review recent logs:

```bash
sudo tail -n 100 \
  "/Library/Logs/fbs-interlock-gateway-cluster/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway-cluster/gateway-error.log"
```

# Firewall and Packet Filter Behavior

The installer applies two macOS firewall controls.

## Application Firewall

The executable is added to the macOS Application Firewall allow list. This control authorizes the application but does not restrict traffic by source IP.

## Managed Packet Filter anchor

The installer creates:

```text
/etc/pf.anchors/com.williamveith.fbs-interlock-gateway-cluster
```

and adds a managed block to:

```text
/etc/pf.conf
```

The generated anchor performs these actions in order:

1. Allows loopback TCP access to ports `8081:8981` for local testing.
2. Allows the configured FBS source IP to TCP ports `8081:8981`.
3. Blocks every other inbound TCP connection to that range.

The current generated values are:

```text
FBS source: 146.6.76.61
Ports:      8081:8981
```

The installer validates the complete candidate `/etc/pf.conf` with:

```bash
sudo pfctl -nf /etc/pf.conf
```

before installing and loading it.

> **Important**
>
> The managed anchor protects the gateway listener range. Keep the Admin UI on `127.0.0.1`; the anchor is not intended to expose or remotely protect the Admin port.

# Gatekeeper and Quarantine

A locally built binary copied by USB normally does not require additional steps. The installer removes the executable quarantine attribute after installing a trusted deployment binary.

Inspect quarantine attributes when troubleshooting:

```bash
xattr -l fbs-interlock-gateway-cluster
```

Remove quarantine recursively only when the files came from the trusted build process:

```bash
xattr -dr com.apple.quarantine .
```

Then rerun the installer.

<div class="page-break"></div>

# Troubleshooting

## Build fails because TLS files are missing

Generate the required runtime files:

```bash
make ca
make gateway-cert
```

Confirm the canonical runtime source files exist:

```bash
ls -l \
  pki/ca/server-ca.crt \
  pki/gateway/gateway-client.crt \
  pki/gateway/gateway-client.key
```

Then rebuild the architecture-specific package.

## Installer reports a binary architecture mismatch

Confirm the gateway architecture:

```bash
uname -m
```

Inspect the packaged binary:

```bash
file fbs-interlock-gateway-cluster
```

Use `build/darwin/arm64/` for `arm64` and `build/darwin/amd64/` for `x86_64`.

## Gateway LaunchDaemon is not running

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-cluster
```

Review logs:

```bash
sudo tail -n 200 \
  "/Library/Logs/fbs-interlock-gateway-cluster/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway-cluster/gateway-error.log"
```

Restart:

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway-cluster
```

## Installer rolls back

The installer rolls back when:

- The generated Packet Filter configuration is invalid
- The gateway LaunchDaemon cannot be loaded
- The Admin API does not become ready within the health-check window
- The update LaunchDaemon cannot be loaded in production mode

Review the installer output and the existing gateway logs. Correct the reported problem and rerun the installer.

## Admin API does not respond

```bash
sudo lsof -nP -iTCP:18090 -sTCP:LISTEN
```

Confirm the Admin UI was not disabled through an empty Admin address.

## Tool listener does not respond

```bash
sudo lsof -nP -iTCP:8081 -sTCP:LISTEN
```

Review the authoritative configuration through the Admin UI or export `gateway.sqlite3`, and confirm the tool is enabled.

## Packet Filter rule is not active

Validate the complete configuration:

```bash
sudo pfctl -nf /etc/pf.conf
```

Display the gateway anchor:

```bash
sudo pfctl \
  -a com.williamveith.fbs-interlock-gateway-cluster \
  -sr
```

Check status:

```bash
sudo pfctl -s info
```

Do not manually replace `/etc/pf.conf` without preserving unrelated system and site rules.

## Update LaunchDaemon is absent

This is expected after a development installation. Restore production updates with:

```bash
sudo ./install.sh
```

Then verify:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-cluster-update
```

## Updater reports another update is running

The updater uses:

```text
/usr/local/libexec/fbs-interlock-gateway-cluster/.update-lock
```

A concurrent updater exits safely. If no update process exists and a stale lock remains after an abnormal termination, inspect the directory before removing it.

## Update fails and rolls back

Review:

```text
/Library/Logs/fbs-interlock-gateway-cluster/update.log
/Library/Logs/fbs-interlock-gateway-cluster/update-error.log
```

Common causes include:

- GitHub Release access failure
- Invalid or missing release checksum
- Wrong release architecture
- Installed binary verification failure
- Gateway Admin API health-check failure after restart

## Shelly hostname does not resolve

```bash
dig +short <shelly-hostname>
```

or:

```bash
nslookup <shelly-hostname>
```

The hostname must match the value in `tools[].ip` and the Shelly server certificate.

## HTTPS or mutual TLS fails

Confirm installed files:

```bash
sudo ls -l \
  "/Library/Application Support/fbs-interlock-gateway-cluster/tls"
```

Verify:

- The Shelly certificate hostname matches `tools[].ip`
- The Shelly trusts `client-ca.crt`
- The gateway trusts `server-ca.crt`
- The gateway client certificate and key match
- The service account can read all configured files
- The system clock is accurate

Look for `phase=tls_handshake` in the gateway logs.

## Admin status appears stale

Ordinary Admin polling reads memory only. A row changes when:

- FBS sends `/status`, `/on`, or `/off` for that tool
- An administrator selects **Refresh Status**

Use explicit refresh for independent verification:

```bash
curl -i \
  "http://127.0.0.1:18090/api/status?refresh=1"
```

# Uninstall the Gateway

Run the uninstaller from the deployment directory.

## Standard uninstall

```bash
sudo ./uninstall.sh
```

Standard uninstall:

- Stops and disables the update LaunchDaemon
- Stops and disables the gateway LaunchDaemon
- Removes both LaunchDaemon property lists
- Removes the executable, startup wrapper, and updater
- Removes the executable from the Application Firewall
- Removes the managed `pf` anchor and managed `/etc/pf.conf` block
- Preserves `gateway.sqlite3`
- Preserves the YAML rollback mirror `config.yaml`
- Preserves installed TLS files
- Preserves gateway and update logs
- Preserves the hidden service account

The preserved authoritative database remains at:

```text
/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3
```

## Purge persistent data

```bash
sudo ./uninstall.sh --purge
```

Purge performs the standard uninstall and also removes:

```text
/Library/Application Support/fbs-interlock-gateway-cluster/
/Library/Logs/fbs-interlock-gateway-cluster/
```

This deletes `gateway.sqlite3`, the YAML rollback mirror, installed TLS files, and logs. The hidden service account is still preserved for safe reinstallation.

> **Destructive operation**
>
> Use `--purge` only after confirming that configuration, certificate, and log retention requirements have been satisfied.

Verify removal:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-cluster

sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-cluster-update
```

Missing-service errors are expected after successful removal.

# Command Reference

| Task | Command |
| --- | --- |
| Detect architecture | `uname -m` |
| Generate gateway TLS material | `make ca && make gateway-cert` |
| Build Apple Silicon package | `make clean && make build-darwin-arm64` |
| Build Intel package | `make clean && make build-darwin-amd64` |
| Production install | `sudo ./install.sh` |
| Development install | `sudo ./install-dev.sh` |
| Check gateway service | `sudo launchctl print system/com.williamveith.fbs-interlock-gateway-cluster` |
| Check updater | `sudo launchctl print system/com.williamveith.fbs-interlock-gateway-cluster-update` |
| Restart gateway | `sudo launchctl kickstart -k system/com.williamveith.fbs-interlock-gateway-cluster` |
| Run updater manually | `sudo /usr/local/libexec/fbs-interlock-gateway-cluster/update.sh` |
| Read Admin cache | `curl -i http://127.0.0.1:18090/api/status` |
| Refresh all tools | `curl -i "http://127.0.0.1:18090/api/status?refresh=1"` |
| Show `pf` anchor | `sudo pfctl -a com.williamveith.fbs-interlock-gateway-cluster -sr` |
| Follow gateway logs | `sudo tail -F "/Library/Logs/fbs-interlock-gateway-cluster/gateway.log" "/Library/Logs/fbs-interlock-gateway-cluster/gateway-error.log"` |
| Follow update logs | `sudo tail -F "/Library/Logs/fbs-interlock-gateway-cluster/update.log" "/Library/Logs/fbs-interlock-gateway-cluster/update-error.log"` |
| Export authoritative config | `sudo /usr/local/libexec/fbs-interlock-gateway-cluster/fbs-interlock-gateway-cluster config export -db "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3" -output /tmp/fbs-interlock-gateway-cluster.yaml` |
| Import edited config | `sudo /usr/local/libexec/fbs-interlock-gateway-cluster/fbs-interlock-gateway-cluster config import -db "/Library/Application Support/fbs-interlock-gateway-cluster/gateway.sqlite3" -input /tmp/fbs-interlock-gateway-cluster.yaml -mirror-config "/Library/Application Support/fbs-interlock-gateway-cluster/config.yaml"` |
| Standard uninstall | `sudo ./uninstall.sh` |
| Purge uninstall | `sudo ./uninstall.sh --purge` |

---

**FBS Interlock Gateway - macOS Installation and Operations Guide**
