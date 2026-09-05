---
title: "FBS Interlock Gateway"
subtitle: "Windows Installation and Operations Guide"
author: "William Veith"
date: "2026-09-04"
lang: en-US
---

> **Purpose**
>
> This guide covers building, transferring, installing, validating, operating, updating, and uninstalling `fbs-interlock-gateway-cluster` on a dedicated 64-bit Windows gateway computer.

> **Security boundary**
>
> The gateway controls software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. The installer restricts the FBS listener range to the authorized FBS source address through Windows Defender Firewall. The Admin UI must remain bound to loopback unless remote access is intentionally secured.

## Table of Contents

- [Deployment Overview](#deployment-overview)
- [Supported Windows Platform and Architecture](#supported-windows-platform-and-architecture)
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
- [Firewall Behavior](#firewall-behavior)
- [Troubleshooting](#troubleshooting)
- [Uninstall the Gateway](#uninstall-the-gateway)
- [Command Reference](#command-reference)

<div class="page-break"></div>

# Deployment Overview

The Windows deployment uses two system-wide Task Scheduler tasks:

1. **FBS Interlock Gateway** starts at system boot and keeps the gateway process running.
2. **FBS Interlock Gateway Update** checks the latest published release once per hour and performs verified binary updates and gateway log maintenance.

The gateway task runs as the restricted built-in `LOCAL SERVICE` account. The update task runs as `SYSTEM` because replacing the installed executable and restarting the gateway require administrative access.

```text
FBS server
    -> Windows Defender Firewall source restriction
    -> Windows gateway listener port
    -> fbs-interlock-gateway-cluster.exe
    -> Shelly RPC over HTTP or HTTPS
    -> tool interlock circuit
```

The installed Admin UI remains local by default:

```text
http://127.0.0.1:18090
```

Normal FBS `/status`, `/on`, and `/off` requests update the Admin status cache for the affected tool. The browser reads that in-memory cache without generating recurring Shelly requests. The **Refresh Status** action performs an explicit fleet-wide verification.

## Deployment sequence

1. Confirm that the gateway computer is running 64-bit Windows.
2. Generate the required gateway runtime TLS files.
3. Build the Windows amd64 deployment directory.
4. Copy the complete deployment directory to the gateway computer.
5. Choose production or development installation mode.
6. Run the selected installer as an administrator.
7. Verify the gateway task, update task, Admin API, TLS files, firewall rule, listener ports, tool communication, and logs.

# Supported Windows Platform and Architecture

The supplied Windows deployment target is:

```text
GOOS=windows
GOARCH=amd64
```

Use it on a 64-bit Windows installation capable of running amd64 executables.

The installer validates all of the following before replacing installed files:

- Windows is 64-bit.
- The deployment executable is a valid PE executable.
- The PE machine type is amd64 (`0x8664`).
- The executable successfully responds to `-version`.

The build target is:

```bash
make build-windows-amd64
```

The generated deployment directory is:

```text
build/windows/
```

> **Important**
>
> Do not mix files from Linux, macOS, or an older Windows deployment directory. Run `make clean` before preparing a fresh Windows package.

# Pre-Deployment Checklist

Before building or installing, confirm the following:

- The repository is on the intended commit or release.
- `make verify` completes successfully.
- The packaged `config.yaml` contains the intended seed configuration for a fresh installation or legacy rollback. After first startup, `gateway.sqlite3` becomes authoritative.
- `make ca` and `make gateway-cert` have populated the required certificate material under `pki/ca/` and `pki/gateway/`.
- The Windows gateway is running a 64-bit version of Windows.
- Windows PowerShell 5.1 is available.
- The `ScheduledTasks` and `NetSecurity` PowerShell modules are available.
- The configured FBS listener ports do not conflict with other services.
- `FBS_SOURCE_IP` and `FBS_PORT_RANGE` in the Makefile are correct before building.
- The Admin address remains `127.0.0.1:18090` unless a reviewed remote-access design is being used.
- The gateway computer can resolve every configured Shelly hostname.
- The gateway computer can reach GitHub Releases when managed automatic updates are required.
- The complete deployment directory will be transferred and stored as one unit.

Current generated firewall defaults are:

```text
Authorized FBS source: 146.6.76.61
Gateway listener range: TCP 8081-8981
```

# Prepare Gateway TLS Material

All Windows deployment builds require the gateway runtime TLS files. The canonical certificate material remains in the PKI directories; the Makefile copies only the runtime files required by the gateway into the deployment package.

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

The Windows build copies these files directly into:

```text
build/windows/tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Do not copy the complete `pki/` directory into the deployment package.

> **Private-key handling**
>
> Never place CA private keys, certificate requests, `client-ca.crt`, or Shelly server private keys on the Windows gateway. Only the gateway runtime server CA certificate, gateway client certificate, and gateway client private key are packaged.

The build fails before producing the Windows deployment directory when any required runtime TLS file is missing.

The default configuration uses relative TLS paths:

```yaml
defaults:
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"
```

The gateway task uses the configured Windows installation directory as its working directory. With the default Makefile settings, these paths resolve under:

```text
C:\FBS\fbs-interlock-gateway-cluster\tls\
```

# Build the Deployment Assets

On the controlled development machine, run:

```bash
make clean
make build-windows-amd64
```

The build performs the following Windows-specific work:

- Verifies the three gateway runtime TLS source files exist.
- Renders the Windows deployment templates.
- Renders the Windows deployment guide as a PDF.
- Copies `config.yaml`.
- Copies the required runtime TLS files from `pki/`.
- Builds the amd64 Windows executable.

The generated directory contains:

```text
build/windows/
├── fbs-interlock-gateway-cluster.exe
├── config.yaml
├── install.bat
├── install-dev.bat
├── install.ps1
├── start.bat
├── update.bat
├── update.ps1
├── uninstall.bat
├── uninstall.ps1
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── Windows Install Instructions.pdf
```

The exact guide PDF name follows the source Markdown filename.

> **Do not edit generated deployment files directly.**
>
> Update the source templates, deployment guide, configuration source, or Makefile variables and rebuild instead.

# Transfer the Deployment Directory

Copy the entire `build/windows/` directory to a USB flash drive or another approved transfer medium.

Do not copy only the executable. The complete directory is required because it contains:

- The application executable
- The production and development launchers
- The PowerShell installer
- The restart supervisor
- The checksum-aware updater
- The uninstaller
- The configuration file
- The gateway runtime TLS files
- The deployment guide PDF

On the Windows gateway computer, copy the complete directory to a local location such as:

```text
C:\Users\<username>\Downloads\windows
```

Confirm that the complete package is present before installation.

Run the installer from the local copy. Do not install directly from removable media.

<div class="page-break"></div>

# Choose an Installation Mode

The deployment package supports two installation modes.

## Production installation

Use the normal installer for a managed production gateway:

```text
install.bat
```

Production mode:

- Installs and starts the gateway task.
- Installs the checksum-aware updater.
- Registers the hourly update task.
- Preserves the authoritative SQLite configuration, YAML rollback/compatibility state, and installed TLS identity.
- Performs gateway log rotation during managed maintenance.
- Restores managed updates after a previous development installation.

## Development installation

Use the development installer when testing an unpublished local build:

```text
install-dev.bat
```

Development mode:

- Installs the local executable and normal gateway task.
- Preserves the authoritative SQLite configuration, YAML rollback/compatibility state, and installed TLS files.
- Removes installed updater scripts.
- Removes the managed update task.
- Prevents the local development executable from being replaced by a published release.

Run the normal production installer later to restore managed automatic updates.

# Install the Gateway

Open the copied Windows deployment directory.

Production installation:

1. Right-click `install.bat`.
2. Select **Run as administrator**.
3. Approve the Windows User Account Control prompt.

Development installation:

1. Right-click `install-dev.bat`.
2. Select **Run as administrator**.
3. Approve the Windows User Account Control prompt.

The batch files are convenience launchers. The equivalent production PowerShell command is:

```powershell
Set-Location C:\Users\<username>\Downloads\windows

PowerShell.exe `
    -NoProfile `
    -ExecutionPolicy Bypass `
    -File .\install.ps1
```

Development mode:

```powershell
PowerShell.exe `
    -NoProfile `
    -ExecutionPolicy Bypass `
    -File .\install.ps1 `
    -Development
```

The installer validates the packaged executable architecture and executes its `-version` command before replacing the existing gateway.

> **Reinstallation behavior**
>
> Reinstallation preserves the authoritative `gateway.sqlite3` database, the YAML rollback mirror, and installed TLS files. It corrects their permissions when required without replacing persistent configuration.

# What the Installer Does

The installer performs the following operations.

## Preflight validation

- Requires administrator privileges.
- Verifies every required deployment file.
- Validates that Windows is 64-bit.
- Validates that the gateway executable is a runnable amd64 PE binary.
- Executes the gateway binary's `-version` command.
- Requires updater files when production mode is selected.
- Captures the existing task definitions and installed files required for rollback.
- Stops the existing gateway and updater tasks before replacement.
- Stops any remaining gateway process.
- Backs up the current `gateway.sqlite3` when present before changing the installation.

## Runtime accounts and permissions

The main gateway task runs as:

```text
NT AUTHORITY\LOCAL SERVICE
```

The installer grants this account the access required for operation:

- Read and execute access to the gateway executable and startup supervisor
- Modify access to the configuration directory because SQLite and the generated YAML rollback mirror use local atomic writes
- Modify access to `config.yaml`
- Modify access to `gateway.sqlite3`
- Read access to the gateway TLS certificate and private-key files
- Modify access to the log directory

The production update task runs as:

```text
SYSTEM
```

Administrative execution is limited to the maintenance workflow because it must download and verify releases, stop and start the gateway task, replace the protected executable, preserve or restore the SQLite database during update rollback, and rotate protected log files.

The installation tree is restricted to:

- Local Administrators: full control
- `SYSTEM`: full control
- `LOCAL SERVICE`: only the runtime access described above

Other ordinary local users are not granted direct access to the authoritative database, YAML rollback mirror, or client private key.

## Application and configuration

- Creates the permanent installation directory.
- Installs `fbs-interlock-gateway-cluster.exe`.
- Installs the `start.bat` restart supervisor.
- Installs updater scripts only in production mode.
- Preserves an existing `gateway.sqlite3`.
- Preserves an existing `config.yaml`.
- Installs the packaged `config.yaml` when no seed/rollback YAML exists.
- Starts the gateway with explicit `-config` and `-db` paths.
- On first successful startup, imports the legacy YAML when the database is uninitialized.
- Treats `gateway.sqlite3` as authoritative after initialization.
- Leaves the original human-authored YAML untouched during first migration.
- Later Admin saves generate a YAML compatibility mirror and preserve the previous YAML as `config.yaml.bak` when possible.

## Gateway TLS files

- Creates the installed `tls\` directory.
- Installs missing runtime TLS files from the deployment package.
- Preserves existing installed TLS files during reinstallation.
- Applies restricted NTFS permissions.
- Grants `LOCAL SERVICE` read access to the installed runtime TLS material.

## Scheduled Tasks

- Registers **FBS Interlock Gateway** under `LOCAL SERVICE`.
- Starts the gateway task after installation.
- Registers **FBS Interlock Gateway Update** under `SYSTEM` only in production mode.
- Removes the managed update task and updater scripts in development mode.
- Waits for the Admin API to become healthy after starting the gateway.
- Verifies that `gateway.sqlite3` exists and is non-empty after startup.
- Reapplies the restricted database ACL after creation.

## Firewall controls

- Sets the Windows Defender Firewall default inbound action to `Block` for Domain, Private, and Public profiles.
- Removes the older unrestricted gateway firewall rule when present.
- Creates a source- and port-restricted inbound allow rule for the gateway executable.
- Restricts the generated listener range to the configured FBS source address.
- Leaves local loopback communication available for the Admin UI and local tool-port testing.

## Logs

Creates:

```text
C:\FBS\fbs-interlock-gateway-cluster\logs\
├── gateway.log
├── gateway-error.log
├── update.log
└── update-error.log
```

The gateway account can modify gateway logs. The update task runs as `SYSTEM` and performs update logging and log rotation.

## Rollback and health validation

The installer captures the existing gateway executable, scripts, task definitions, and authoritative SQLite database before replacing an existing installation.

If installation fails after replacement, the installer removes transient SQLite sidecar files, restores the prior `gateway.sqlite3` when it existed, restores the previous managed executable and task state, and reapplies the database ACL when possible.

The preserved YAML rollback mirror and installed TLS files are not replaced during normal reinstallation.

# Installed Layout

## Application and persistent configuration directory

With the default Makefile settings:

```text
C:\FBS\fbs-interlock-gateway-cluster\
├── fbs-interlock-gateway-cluster.exe
├── gateway.sqlite3          # authoritative SQLite configuration
├── config.yaml              # first-run seed; later generated rollback mirror
├── config.yaml.bak          # previous YAML mirror when available
├── start.bat
├── update.bat               # production mode only
├── update.ps1               # production mode only
├── tls\
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── logs\
    ├── gateway.log
    ├── gateway-error.log
    ├── update.log
    └── update-error.log
```

After `gateway.sqlite3` is initialized, manual edits to `config.yaml` are ignored by normal gateway startup. Use the Admin UI or the documented export/import workflow to change the authoritative configuration.

## Scheduled Tasks

Production mode registers:

```text
FBS Interlock Gateway
FBS Interlock Gateway Update
```

A development installation does not retain the managed update task.

Timestamped executable backups created by successful update attempts are stored beside the executable:

```text
fbs-interlock-gateway-cluster.exe.backup.YYYYMMDDTHHMMSSZ
```

<div class="page-break"></div>

# Verify the Installation

Open PowerShell as an administrator.

## Check the installed version

```powershell
& "C:\FBS\fbs-interlock-gateway-cluster\fbs-interlock-gateway-cluster.exe" `
    -version
```

## Check the gateway task

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway"
```

View detailed task information:

```powershell
Get-ScheduledTaskInfo `
    -TaskName "FBS Interlock Gateway"
```

Confirm the runtime account:

```powershell
(Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway").Principal
```

The user ID should resolve to `LOCAL SERVICE`.

While the gateway executable is active, the task should report a running state.

## Check the update task

Production installation:

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway Update"
```

A development installation should not have this task:

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway Update" `
    -ErrorAction SilentlyContinue
```

## Check the Admin API

Read the current in-memory status snapshot:

```powershell
Invoke-WebRequest `
    -UseBasicParsing `
    http://127.0.0.1:18090/api/status
```

A successful response should report HTTP status `200`.

Start an explicit fleet refresh:

```powershell
Invoke-WebRequest `
    -UseBasicParsing `
    "http://127.0.0.1:18090/api/status?refresh=1"
```

The response headers expose the current refresh state through:

```text
X-Status-Refresh-In-Progress
```

## Confirm listening ports

Admin port:

```powershell
Get-NetTCPConnection `
    -LocalPort 18090 `
    -State Listen `
    -ErrorAction SilentlyContinue
```

Example FBS listener port:

```powershell
Get-NetTCPConnection `
    -LocalPort 8081 `
    -State Listen `
    -ErrorAction SilentlyContinue
```

Display the complete configured gateway range:

```powershell
Get-NetTCPConnection `
    -State Listen `
    -ErrorAction SilentlyContinue |
    Where-Object {
        $_.LocalPort -eq 18090 -or
        ($_.LocalPort -ge 8081 -and $_.LocalPort -le 8981)
    } |
    Sort-Object LocalPort
```

## Verify configuration database, rollback mirror, and TLS permissions

Confirm the required persistent files exist:

```powershell
Get-Item `
    C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3

Get-Item `
    C:\FBS\fbs-interlock-gateway-cluster\config.yaml

Get-ChildItem `
    C:\FBS\fbs-interlock-gateway-cluster\tls
```

`gateway.sqlite3` should be non-empty. Expected TLS files are:

```text
server-ca.crt
gateway-client.crt
gateway-client.key
```

Inspect the database and private-key ACLs:

```powershell
Get-Acl `
    C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3 |
    Format-List

Get-Acl `
    C:\FBS\fbs-interlock-gateway-cluster\tls\gateway-client.key |
    Format-List
```

The ACLs should retain full access for Administrators and `SYSTEM`. `LOCAL SERVICE` requires modify access to the SQLite database/configuration directory and read access to the TLS material.

## Verify Windows Defender Firewall

Inspect the managed rule:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Format-List
```

Inspect the port filter:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallPortFilter |
    Format-List
```

Inspect the remote-address filter:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallAddressFilter |
    Format-List
```

Confirm that the generated rule restricts inbound TCP traffic to the configured listener range and authorized FBS source.

# Verify Tool Communication

Replace `8081` with the configured listener port for the tool being tested.

Read the tool status:

```powershell
Invoke-RestMethod `
    http://127.0.0.1:8081/status
```

Turn the tool output on:

```powershell
Invoke-RestMethod `
    http://127.0.0.1:8081/on
```

Turn the tool output off:

```powershell
Invoke-RestMethod `
    http://127.0.0.1:8081/off
```

Expected FBS-compatible responses include:

```json
{"Success":1,"State":1}
```

and:

```json
{"Success":1,"State":0}
```

> **Operational caution**
>
> `/on` and `/off` operate the configured interlock. Perform command testing only when changing the interlock state is authorized and safe.

## Verify HTTPS or mutual TLS directly

For an HTTPS or mutual-TLS Shelly configuration, first confirm that the gateway can resolve and reach the Shelly hostname:

```powershell
Resolve-DnsName <shelly-ddns-host>

Test-NetConnection `
    <shelly-ddns-host> `
    -Port 443
```

Then initiate a normal gateway status request for that tool and review the gateway error log for certificate, hostname, trust-chain, or client-certificate failures:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\gateway-error.log `
    -Tail 100
```

A successful gateway `/status` request without a corresponding TLS error validates the installed runtime trust path in the same process that communicates with the Shelly.

# View the Admin Panel

On the gateway machine, open:

<http://127.0.0.1:18090>

The Admin UI provides:

- Current cached status for every configured tool
- Connected, disconnected, output, protocol, and error information
- Passive row updates from normal FBS traffic
- A **Refresh Status** button for explicit fleet verification
- Configuration editing and validation
- Password replacement or clearing without exposing stored passwords
- A gateway restart after a successful configuration save

## Passive status behavior

The browser polls only the gateway's in-memory status cache every three seconds. These cache reads do not contact Shelly devices.

```text
FBS request
    -> Shelly operation
    -> shared status row updated
    -> Admin browser reads cache
```

Use **Refresh Status** when an independent `Switch.GetStatus` check is required.

An explicit refresh performs the fleet verification through the gateway rather than through the browser directly.

## Remote Admin access

Keep the Admin UI bound to loopback.

Do not expose port `18090` through Windows Defender Firewall without a reviewed authentication, authorization, and transport-security design.

If Windows OpenSSH Server is intentionally configured for administrative access, use an SSH tunnel from an authorized computer rather than changing the Admin bind address:

```bash
ssh -L 18090:127.0.0.1:18090 \
  <windows-user>@<gateway-host>
```

Then open locally:

```text
http://127.0.0.1:18090
```

# Automatic Updates and Log Maintenance

Production installation creates:

```text
FBS Interlock Gateway Update
```

The task starts at minute `17` and repeats once per hour.

## Update behavior

The updater:

1. Requires administrative execution through the `SYSTEM` update task.
2. Acquires an update lock to prevent overlapping update operations.
3. Downloads the Windows AMD64 `.sha256` file and detached `.sha256.sig`.
4. Uses the currently installed gateway binary to authenticate the signed checksum and asset name.
5. Calculates the installed executable SHA-256 checksum.
6. Skips the executable download when the installed checksum already matches and no log maintenance is required.
7. Downloads the release executable only when the authenticated checksum differs.
8. Verifies the downloaded SHA-256 checksum and amd64 PE format.
9. Runs the downloaded executable with `-version` and rejects authenticated downgrades.
10. Backs up the current executable and, when a replacement is required, the authoritative `gateway.sqlite3` plus its original ACL.
11. Stops the gateway task only when replacement or log maintenance requires it.
12. Rotates oversized gateway logs.
13. Stages and verifies the replacement executable.
14. Starts the gateway task.
15. Waits for the Admin API and verifies that the SQLite database remains present and non-empty.
16. Restores both the previous executable and previous SQLite database/ACL when the post-update health or database check fails.

The updater does not intentionally replace the YAML rollback mirror or installed TLS files:

- `config.yaml`
- `server-ca.crt`
- `gateway-client.crt`
- `gateway-client.key`

A new binary can open or migrate the authoritative database during startup. The database backup is therefore part of binary-update rollback.

## Log rotation

During each hourly maintenance check, the updater examines:

```text
gateway.log
gateway-error.log
```

A log is rotated after it reaches `10 MiB`.

The updater retains up to 30 compressed archives:

```text
gateway.log.1.zip
gateway.log.2.zip
...
gateway.log.30.zip
```

The same retention pattern applies to `gateway-error.log`.

## Run maintenance manually

Open PowerShell as an administrator:

```powershell
Start-ScheduledTask `
    -TaskName "FBS Interlock Gateway Update"
```

Follow the updater log:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\update.log `
    -Wait
```

## Disable managed updates for development

Use the packaged development installer:

```text
Right-click install-dev.bat -> Run as administrator
```

The development installer removes the update task and installed updater scripts.

## Restore managed updates

Run the normal production installer:

```text
Right-click install.bat -> Run as administrator
```

# View Gateway Logs

Follow gateway standard output:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\gateway.log `
    -Wait
```

Follow gateway errors:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\gateway-error.log `
    -Wait
```

Follow updater output:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\update.log `
    -Wait
```

Follow updater errors:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\update-error.log `
    -Wait
```

Press `Ctrl+C` to stop following a log.

# Edit the Configuration

The authoritative configuration is:

```text
C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3
```

The compatibility/rollback YAML is:

```text
C:\FBS\fbs-interlock-gateway-cluster\config.yaml
```

`config.yaml` is used as an import source only while the SQLite database is uninitialized. Once `gateway.sqlite3` contains configuration, normal startup loads SQLite and ignores manual edits to the YAML file.

## Preferred method: Admin UI

Open:

```text
http://127.0.0.1:18090
```

The Admin UI validates the complete proposed configuration, preserves stored passwords unless explicitly replaced or cleared, commits the change transactionally to SQLite, writes a generated YAML compatibility mirror when possible, preserves the previous YAML as `config.yaml.bak`, and requests a clean gateway restart.

## Manual method: export, edit, and import

Open PowerShell as an administrator.

Export the authoritative database:

```powershell
& 'C:\FBS\fbs-interlock-gateway-cluster\fbs-interlock-gateway-cluster.exe' `
    config export `
    -db 'C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3' `
    -output "$env:TEMP\fbs-interlock-gateway-cluster.yaml"
```

Edit the exported YAML:

```powershell
Start-Process notepad.exe `
    -ArgumentList "$env:TEMP\fbs-interlock-gateway-cluster.yaml" `
    -Verb RunAs `
    -Wait
```

Import the complete edited configuration transactionally and refresh the YAML rollback mirror:

```powershell
& 'C:\FBS\fbs-interlock-gateway-cluster\fbs-interlock-gateway-cluster.exe' `
    config import `
    -db 'C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3' `
    -input "$env:TEMP\fbs-interlock-gateway-cluster.yaml" `
    -mirror-config 'C:\FBS\fbs-interlock-gateway-cluster\config.yaml'
```

Restart the gateway task after a successful CLI import:

```powershell
Stop-ScheduledTask `
    -TaskName "FBS Interlock Gateway"

Start-ScheduledTask `
    -TaskName "FBS Interlock Gateway"
```

For review or sharing, create a redacted export:

```powershell
& 'C:\FBS\fbs-interlock-gateway-cluster\fbs-interlock-gateway-cluster.exe' `
    config export `
    -db 'C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3' `
    -output "$env:TEMP\fbs-interlock-gateway-cluster-redacted.yaml" `
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

Open PowerShell as an administrator:

```powershell
Stop-ScheduledTask `
    -TaskName "FBS Interlock Gateway"

Start-ScheduledTask `
    -TaskName "FBS Interlock Gateway"
```

Verify the Admin API:

```powershell
Invoke-RestMethod `
    http://127.0.0.1:18090/api/status
```

The `start.bat` supervisor provides bounded restart behavior around the gateway executable:

- Restart delay: 2 seconds
- Restart window: 60 seconds
- Maximum starts within the window: 10
- Continuous restart after normal or abnormal executable exit until the rate limit is exceeded

Task Scheduler also provides a slower fallback restart policy if the supervisor itself exits.

# Firewall Behavior

The installer sets the default inbound action to `Block` for the Domain, Private, and Public Windows Defender Firewall profiles.

It creates this application-specific allow rule:

```text
Display name: FBS Interlock Gateway - Authorized FBS Source
Direction:    Inbound
Protocol:     TCP
Program:      C:\FBS\fbs-interlock-gateway-cluster\fbs-interlock-gateway-cluster.exe
Local ports:  8081-8981
Remote IP:    146.6.76.61
Profiles:     Any
```

The installer removes the older broad rule named:

```text
FBS Interlock Gateway
```

Inspect the active rule:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Format-List
```

Inspect its port filter:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallPortFilter |
    Format-List
```

Inspect its remote-address filter:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallAddressFilter |
    Format-List
```

Local loopback access remains available for the Admin UI and local tool-port testing.

<div class="page-break"></div>

# Troubleshooting

## Build fails because TLS files are missing

Confirm the development repository contains:

```text
pki/ca/server-ca.crt
pki/gateway/gateway-client.crt
pki/gateway/gateway-client.key
```

If the PKI has not been created, generate it:

```bash
make ca
make gateway-cert
```

Then rebuild:

```bash
make clean
make build-windows-amd64
```

Do not bypass the build check by manually assembling an incomplete deployment directory.

## The executable architecture does not match the gateway

Rebuild with:

```bash
make clean
make build-windows-amd64
```

Confirm the deployment directory was not mixed with a Linux or macOS binary.

## The gateway task is not running

Inspect task state and history:

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway"

Get-ScheduledTaskInfo `
    -TaskName "FBS Interlock Gateway"
```

Review the gateway error log:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\gateway-error.log `
    -Tail 100
```

Start the task manually:

```powershell
Start-ScheduledTask `
    -TaskName "FBS Interlock Gateway"
```

A task that returns to `Ready` immediately may indicate that the supervisor or gateway executable exited. Review the logs before repeatedly restarting it.

## The Admin API does not respond

Check whether the gateway task is running:

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway"
```

Confirm port `18090` is listening:

```powershell
Get-NetTCPConnection `
    -LocalPort 18090 `
    -State Listen `
    -ErrorAction SilentlyContinue
```

Review:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\gateway-error.log `
    -Tail 100
```

## A tool listener does not respond

Confirm the configured listener port is open locally:

```powershell
Test-NetConnection `
    127.0.0.1 `
    -Port 8081
```

Check the authoritative configuration through the Admin UI or export `gateway.sqlite3`, then review gateway logs for hostname resolution, authentication, timeout, TLS, or certificate errors.

## The gateway cannot read the database, rollback mirror, or TLS files

Inspect the ACLs:

```powershell
Get-Acl `
    C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3 |
    Format-List

Get-Acl `
    C:\FBS\fbs-interlock-gateway-cluster\config.yaml |
    Format-List

Get-Acl `
    C:\FBS\fbs-interlock-gateway-cluster\tls\gateway-client.key |
    Format-List
```

Re-run `install.bat` as administrator to restore the intended database/configuration permissions without replacing the authoritative configuration or TLS identity.

## The Windows Defender Firewall rule is missing or incorrect

Inspect the rule:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" `
    -ErrorAction SilentlyContinue
```

Confirm the source address:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallAddressFilter
```

Confirm the listener range:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallPortFilter
```

Rebuild and reinstall after changing `FBS_SOURCE_IP` or `FBS_PORT_RANGE` in the Makefile.

## The update task is absent

A development installation intentionally removes the managed update task.

For production mode, check:

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway Update" `
    -ErrorAction SilentlyContinue
```

Run the normal production installer again to restore managed updates.

## The updater reports a checksum or download failure

Do not install the downloaded file manually.

Review:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\update-error.log `
    -Tail 100
```

A checksum mismatch causes the updater to stop before replacing the installed executable.

Also confirm that the gateway computer can reach the published GitHub release assets.

## The update is rolled back

The updater restores the previous executable when the Admin API does not become healthy after replacement.

Review:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\update-error.log `
    -Tail 100

Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\gateway-error.log `
    -Tail 100
```

Timestamped backup executables remain in the installation directory for inspection.

## A Shelly hostname does not resolve

Test the configured hostname:

```powershell
Resolve-DnsName `
    2c41389b0d77.dynamic.utexas.edu
```

Test HTTPS reachability:

```powershell
Test-NetConnection `
    2c41389b0d77.dynamic.utexas.edu `
    -Port 443
```

The `tools[].ip` field must contain only the hostname or IP address.

## HTTPS or mutual TLS fails

Confirm the installed TLS files exist:

```powershell
Get-ChildItem `
    C:\FBS\fbs-interlock-gateway-cluster\tls
```

Check the gateway error log for:

- Certificate expiration
- Hostname mismatch
- Unknown certificate authority
- Client-certificate rejection
- TLS handshake timeout
- Incorrect Shelly hostname

Confirm that the Shelly server certificate is signed by the server CA installed on the gateway and that the Shelly trusts the client CA that signed `gateway-client.crt`.

## A request reaches its context deadline

Review gateway errors:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway-cluster\logs\gateway-error.log `
    -Tail 100
```

Confirm:

- The Shelly hostname resolves quickly.
- Port `443` is reachable when HTTPS is configured.
- Authentication credentials are correct.
- The TLS handshake is not failing or repeatedly retrying.
- The configured request timeout is appropriate for the deployment.

## Admin status appears stale

Normal browser polling reads the in-memory status cache and does not force a Shelly request.

Use **Refresh Status** in the Admin UI when an independent fleet verification is required.

You can also request a refresh directly:

```powershell
Invoke-RestMethod `
    "http://127.0.0.1:18090/api/status?refresh=1"
```

# Uninstall the Gateway

The uninstaller removes managed executable files and operating-system integration while preserving persistent gateway data by default.

## Standard uninstall

Open the original Windows deployment directory.

Right-click:

```text
uninstall.bat
```

Select:

```text
Run as administrator
```

Standard uninstall removes:

- Gateway task
- Update task
- Running gateway process
- Windows Defender Firewall rules
- Application executable
- Startup supervisor
- Updater scripts
- Executable backups

It preserves:

```text
C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3
C:\FBS\fbs-interlock-gateway-cluster\config.yaml
C:\FBS\fbs-interlock-gateway-cluster\tls\
C:\FBS\fbs-interlock-gateway-cluster\logs\
```

`gateway.sqlite3` remains the authoritative configuration; `config.yaml` remains the first-run/rollback compatibility copy.

## Purge persistent configuration, SQLite state, TLS, and logs

A purge also removes the authoritative database, YAML rollback mirror, TLS files, and logs.

Open an elevated Command Prompt in the deployment directory:

```bat
uninstall.bat --purge
```

Equivalent PowerShell command:

```powershell
PowerShell.exe `
    -NoProfile `
    -ExecutionPolicy Bypass `
    -File .\uninstall.ps1 `
    -Purge
```

> **Destructive operation**
>
> Purge deletes `gateway.sqlite3`, the YAML rollback mirror, the installed gateway client private key, and local logs. Confirm that required configuration, certificate, and log retention requirements have been satisfied before using it.

Verify that the managed tasks were removed:

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway" `
    -ErrorAction SilentlyContinue

Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway Update" `
    -ErrorAction SilentlyContinue
```

No matching tasks should remain after a successful uninstall.

# Command Reference

| Task | Command |
| --- | --- |
| Generate gateway TLS material | `make ca && make gateway-cert` |
| Build Windows package | `make clean && make build-windows-amd64` |
| Production install | Right-click `install.bat` -> **Run as administrator** |
| Development install | Right-click `install-dev.bat` -> **Run as administrator** |
| Check gateway task | `Get-ScheduledTask -TaskName "FBS Interlock Gateway"` |
| Check updater task | `Get-ScheduledTask -TaskName "FBS Interlock Gateway Update"` |
| Restart gateway | `Stop-ScheduledTask -TaskName "FBS Interlock Gateway"; Start-ScheduledTask -TaskName "FBS Interlock Gateway"` |
| Run updater manually | `Start-ScheduledTask -TaskName "FBS Interlock Gateway Update"` |
| Read Admin cache | `Invoke-RestMethod http://127.0.0.1:18090/api/status` |
| Refresh all tools | `Invoke-RestMethod "http://127.0.0.1:18090/api/status?refresh=1"` |
| Show firewall rule | `Get-NetFirewallRule -DisplayName "FBS Interlock Gateway - Authorized FBS Source"` |
| Follow gateway logs | `Get-Content C:\FBS\fbs-interlock-gateway-cluster\logs\gateway.log -Wait` |
| Follow gateway errors | `Get-Content C:\FBS\fbs-interlock-gateway-cluster\logs\gateway-error.log -Wait` |
| Follow update logs | `Get-Content C:\FBS\fbs-interlock-gateway-cluster\logs\update.log -Wait` |
| Export authoritative config | `& 'C:\FBS\fbs-interlock-gateway-cluster\fbs-interlock-gateway-cluster.exe' config export -db 'C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3' -output "$env:TEMP\fbs-interlock-gateway-cluster.yaml"` |
| Import edited config | `& 'C:\FBS\fbs-interlock-gateway-cluster\fbs-interlock-gateway-cluster.exe' config import -db 'C:\FBS\fbs-interlock-gateway-cluster\gateway.sqlite3' -input "$env:TEMP\fbs-interlock-gateway-cluster.yaml" -mirror-config 'C:\FBS\fbs-interlock-gateway-cluster\config.yaml'` |
| Standard uninstall | Right-click `uninstall.bat` -> **Run as administrator** |
| Purge uninstall | `uninstall.bat --purge` |

---

**FBS Interlock Gateway - Windows Installation and Operations Guide**
