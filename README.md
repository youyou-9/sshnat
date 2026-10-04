# SSHNat

<div align="center">

**A Modern Lightweight SSH Port Forwarding Desktop Client & Headless Daemon**  
*现代轻量级 SSH 端口转发客户端与无头后台守护进程*

[![Build Status](https://github.com/youyou-9/sshnat/actions/workflows/build.yml/badge.svg)](https://github.com/youyou-9/sshnat/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26.6+-00ADD8?logo=go)]()
[![Wails Version](https://img.shields.io/badge/Wails-v3.0.0--beta.11-DF0000?logo=wails)]()
[![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte)]()
[![License](https://img.shields.io/badge/License-MIT-blue.svg)]()

[🇬🇧 English Documentation](README.md) · [🇨🇳 简体中文](README_zh.md)

</div>

---

## Table of Contents
- [📖 Introduction](#-introduction)
- [✨ Key Features](#-key-features)
- [🏗️ Project Architecture](#-project-architecture)
- [🚀 Quick Start & Building](#-quick-start--building)
  - [Prerequisites](#prerequisites)
  - [Building Desktop App](#building-desktop-app)
  - [Building Headless Daemon](#building-headless-daemon)
- [📁 Portable Mode & Config Layout](#-portable-mode--config-layout)
- [🔐 Advanced Host Settings](#-advanced-host-settings)
- [🖥️ System Tray Capabilities](#-system-tray-capabilities)
- [🧪 Testing & Quality Gate](#-testing--quality-gate)
- [📄 License](#-license)

---

### 📖 Introduction
**SSHNat** is a modern, lightweight, and deeply decoupled SSH port forwarding manager and background daemon. It combines Go's native concurrent networking engine with Svelte 5 (Runes) and Tailwind CSS v4 on top of Wails v3. It also provides a standalone headless CLI daemon (`sshnatd`) tailored for server environments, automated pipelines, and developer workflows.

---

### ✨ Key Features
- **All-in-One Port Forwarding**:
  - **Local Forwarding (`-L`)**: Expose remote internal services or server-local services locally over SSH (supports both TCP and Unix domain sockets).
  - **Remote Forwarding (`-R`)**: Listen on the remote SSH server and forward incoming connections back to local endpoints.
  - **Dynamic Proxy (`-D`)**: High-performance local SOCKS5 proxy server.
- **Versatile Authentication**:
  - Passwords, passphrase-protected private keys, system and third-party SSH Agents (Windows named pipes `\\.\pipe\openssh-ssh-agent` / `1password-ssh-agent`, Unix domain sockets).
  - Multi-hop jump hosts (`ProxyJump`) with cycle detection.
- **Resilient Lifecycle Management**:
  - Native OpenSSH keepalive probing and exponential backoff auto-reconnection.
  - Bidirectional stream co-termination preventing orphan socket and file descriptor leaks.
- **OpenSSH Command Import and Copy**:
  - Paste supported OpenSSH forwarding commands (`ssh -L ... -i ... -p ... user@host`) for instant import.
  - Automatically synthesizes compliant, copy-ready OpenSSH commands with proper escaping.
- **Real-Time Telemetry & Granular Diagnostics**:
  - Real-time TX/RX speeds, cumulative bandwidth counters, and 60-point SVG Sparkline charts.
  - Live streaming handshake, target dialing, and failure logs (e.g. Connection Refused, Timeout).
  - Dedicated per-tunnel log modal for distraction-free troubleshooting.
- **Background Tray & Dynamic Quick Switcher**:
  - Minimizing or closing the window parks the app in the system tray without dropping tunnels.
  - Right-click tray menu dynamically displays recently used tunnels sorted by activity, allowing background toggling without opening the window.
- **Portable Configuration Mode**:
  - Automatically prioritizes `config.json` next to the executable; falls back safely to OS standard user directories in read-only locations.
- **Bilingual & Modern Theming**:
  - Dark / Light / System auto-adaptation, with 100% full English and Chinese localization.

---

### 🏗️ Project Architecture

```text
sshnat/
├── app/                  # Wails v3 service layer: RPC bindings, validation, events & CLI parser
├── build/                # Cross-platform packaging assets (macOS .app, Linux NFPM, Windows, Docker)
├── cmd/
│   └── sshnatd/          # Headless background CLI daemon entry point
├── core/                 # Core network forwarding engine (Pure Go, 0 GUI dependencies)
│   ├── config/           # Models, Store atomic file persistence, CLI parser & portable locator
│   ├── forward/          # Forwarders (local.go, remote.go, dynamic.go, counting & copy buffers)
│   ├── internal/         # Cross-platform SSH-Agent drivers (Windows named pipes / Unix sockets)
│   ├── ssh/              # Client lifecycle, multi-hop ProxyJump, auth signers & known_hosts
│   ├── stats/            # Concurrent atomic traffic and connection counters
│   └── supervisor/       # Supervisor state machine, backoff loops, stats ticker & event bus
├── frontend/             # Desktop frontend (Svelte 5 Runes + Vite + Tailwind CSS v4)
│   ├── bindings/         # Auto-generated TypeScript bindings by Wails v3
│   └── src/              # Views (Dashboard Cockpit, Tunnels Control Center, Logs, Hosts, Settings)
├── main.go               # Desktop GUI entry point (Wails v3 assembly & Tray controller)
├── go.mod / go.sum       # Go module definitions
├── README.md             # English documentation (Default)
├── README_zh.md          # Chinese documentation
└── Taskfile.yml          # Build and packaging automation recipes
```

---

### 🚀 Quick Start & Building

#### Prerequisites
- **Go**: 1.26.6+ (includes the security fixes required by the release checks)
  - For optional obfuscated builds, install Go 1.26.6 directly from [go.dev/dl](https://go.dev/dl/). Garble cannot patch an automatically downloaded toolchain inside Go's module cache.
- **Node.js**: 24.15+ & npm (the locked Vite/Vitest/jsdom toolchain requires Node 24.15+)
- **Wails 3 CLI**:
  ```bash
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.11
  ```

#### Building Desktop App
```bash
# 1. Clone the repository
git clone https://github.com/youyou-9/sshnat.git
cd sshnat

# 2. Install frontend dependencies
cd frontend && npm ci && cd ..

# 3. Live development mode
wails3 task dev

# 4. Production build
wails3 task build
```
The output binary will be generated under `bin/` (`bin/sshnat.exe` on Windows).

*Note for Linux builds:*
```bash
# Debian 13+ / Ubuntu 24.04+ (default GTK4 renderer)
sudo apt install -y build-essential libgtk-4-dev libwebkitgtk-6.0-dev pkg-config

# Fedora / RHEL
sudo dnf install -y gtk4-devel webkitgtk6.0-devel
```
The default renderer needs GTK 4.14+. For a distribution with GTK3/WebKitGTK
4.1, install its development packages and build with
`wails3 task build EXTRA_TAGS=gtk3` (the compatibility renderer in Wails 3.0).
Use the same `EXTRA_TAGS=gtk3` when creating Linux packages, for example
`wails3 task linux:create:deb EXTRA_TAGS=gtk3 VERSION=1.0.0`. The deb/rpm/Arch
tasks automatically select the matching runtime dependencies and preserve
the binary's executable permissions.

#### Building Headless Daemon
```bash
# Local builds are labelled "dev" in the About panel and window title.
go build -o bin/sshnatd ./cmd/sshnatd

# Build all release daemon targets with a display version (requires Taskfile)
wails3 task daemon:all VERSION=1.0.0
```
Run all configured tunnels:
```bash
./bin/sshnatd -config ./config.json -all
```
Check the installed daemon version without reading a configuration file:
```bash
./bin/sshnatd -version
```
Validate a configuration without opening any network connection:
```bash
./bin/sshnatd -config ./config.json -check
```
`-check` returns nonzero for missing files, unsupported fields, invalid hosts
or tunnels, broken references, and ProxyJump cycles. It accepts a valid empty
configuration; normal daemon startup requires at least one selected tunnel.

Build a minimal container image (the image contains only `sshnatd`, without a
GUI or HTTP listener):
```bash
wails3 task common:build:docker TAG=sshnat:1.0.0 VERSION=1.0.0
docker run --rm \
  --mount "type=bind,source=$PWD/sshnat-config,target=/config" \
  sshnat:1.0.0 -config /config/config.json -all
```

Put `config.json` in the mounted directory; `known_hosts` entries are also kept
there across restarts. Private-key paths in the config must point to files
available inside the container. To expose a local TCP/SOCKS listener through
Docker's port mapping, set its `localBindHost` to `0.0.0.0` and add the matching
`-p HOST_PORT:CONTAINER_PORT` option. The daemon exits nonzero for missing
configuration or an invalid tunnel selection. Docker reports process exit
through the normal container lifecycle; endpoint availability should be
monitored using the actual forwarded service.

Release builds inject the version from a `v*` tag. For local reproducible
builds, pass `VERSION=1.0.0` to the Taskfile commands. Desktop build tasks also
synchronize native package metadata (Windows file info/MSIX/NSIS, macOS plist,
Linux NFPM) using the numeric version. Prerelease suffixes remain visible in
the application and daemon version output.

---

### 📁 Portable Mode & Config Layout
- **Portable Priority**: When the directory containing the binary is writable, `config.json` and `known_hosts` are kept directly beside the executable.
- **Safety Fallback**: When placed in read-only directories (e.g. Linux `/usr/bin/` or macOS `.app/Contents/MacOS/`), configuration falls back automatically to:
  - Windows: `%APPDATA%\sshnat\config.json`
  - Linux: `~/.config/sshnat/config.json`
  - macOS: `~/Library/Application Support/sshnat/config.json`
- **Path Expansion**: Private key and known-hosts paths support `~` (e.g. `~/.ssh/id_ed25519`), which expands across Windows, Linux, and macOS.

---

### 🔐 Advanced Host Settings

Open **Hosts → Add/Edit → Advanced settings** to configure:

| Setting | Behavior |
| --- | --- |
| Connection timeout | 1–300 seconds; default 15. In JSON, `connectTimeoutSeconds: 0` also selects the default. |
| Keepalive interval | `0` uses the 15-second default, `-1` disables probing, and `1`–`86400` sets an explicit interval in seconds. |
| Host key policy | `accept-new` records unknown keys; `strict` requires an existing matching record. Both reject a changed host key. |
| Known-hosts file | Empty uses `known_hosts` beside the active `config.json`; an explicit path can point to a trusted OpenSSH file. Strict mode never creates this file. |
| Jump host chain | Add, remove or reorder saved hosts from nearest to farthest. Each hop retains its own authentication, host key policy and timeout. Cycles are rejected when saving. |

New and existing configurations default to `accept-new` when `hostKeyPolicy`
is omitted. Before using `strict`, provision the selected known-hosts file
with the server's verified public key. A failed test reports the handshake or
host key error without changing the policy.

Copied commands include the destination's host key policy, connection timeout,
keepalive, explicit known-hosts path and agent socket through `-o`. Import accepts
case-insensitive option names and both `Name=value` and quoted `Name value` forms.
`ServerAliveInterval=0` maps to the app's disabled keepalive (`-1`); app defaults
(`0` or omitted) copy as 15 seconds. Import rejects `StrictHostKeyChecking=no/off/ask`,
unlimited `ConnectTimeout=0`, multiple known-hosts files, and combined `-i` plus
`IdentityAgent`, because these settings cannot be represented by the app.

An empty known-hosts path uses `known_hosts` beside the active `config.json` in
SSHNat, while OpenSSH uses its own default `~/.ssh/known_hosts`. Set an explicit
path in the host settings to reuse the same trusted fingerprints in copied
commands. `-J` carries jump addresses, users and ports; it cannot carry each
jump's independent keys, agent sockets, host key policies or timeouts. Configure
these per jump in your OpenSSH config before running a copied multi-hop command.
SSH config files supplied with `-F` are not loaded by command import. Use a full
configuration backup for transferring all app settings and saved credentials.

---

### Configuration migration and backups

Settings can save or copy configuration JSON and import a file or pasted JSON.
Exports omit passwords and key passphrases by default; enable **Full backup**
when moving credentials to a private location. Private key files and
`known_hosts` must be copied separately.

- **Merge** keeps existing entries and assigns new IDs to imported hosts and
  tunnels, preserving their jump references.
- **Replace** requires every tunnel to be stopped and saves the exact previous
  configuration as `config-backup-*.json` beside the active file. Import that
  backup to restore it.
- Imported tunnels remain stopped until started manually; their auto-start
  setting applies the next time the app opens.
- Unknown fields, unsupported versions, invalid references, and jump cycles
  are rejected before importing. Legacy documents without a version use schema 1.

### 🖥️ System Tray Capabilities
- **Background Daemon**: Closing the window hides it into the tray, ensuring active forwarders remain uninterrupted.
- **Quick Switcher**: The tray menu dynamically presents the most recently used tunnels as interactive checkboxes, enabling one-click background toggling.
- **Global Actions**: Start All, Stop All, Open Config Folder, and Quit.

---

### 🧪 Testing & Quality Gate
```bash
# Run all Go core and end-to-end tests
go test -v ./...

# Run frontend Vitest suite
cd frontend && npm test
```

See [Release checklist](RELEASE_CHECKLIST.md) for build, runtime and packaging gates.

---

### 📄 License
This project is open-source software licensed under the [MIT License](LICENSE).
