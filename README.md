# SSHNat

<div align="center">

**A Modern Lightweight SSH Port Forwarding Desktop Client & Headless Daemon**  
*现代轻量级 SSH 端口转发客户端与无头后台守护进程*

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)]()
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
- **Bi-directional OpenSSH CLI Parity**:
  - Paste any standard OpenSSH command line (`ssh -L ... -i ... -p ... user@host`) for instant import.
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
- **Go**: 1.25+
- **Node.js**: 18+ & npm
- **Wails 3 CLI**:
  ```bash
  go install github.com/wailsapp/wails/v3/cmd/wails3@latest
  ```

#### Building Desktop App
```bash
# 1. Clone the repository
git clone https://github.com/youyou-9/sshnat.git
cd sshnat

# 2. Install frontend dependencies
cd frontend && npm install && cd ..

# 3. Live development mode
wails3 task dev

# 4. Production build
wails3 task build
```
The output binary will be generated under `bin/` (`bin/sshnat.exe` on Windows).

*Note for Linux builds:*
```bash
# Debian / Ubuntu
sudo apt install -y build-essential libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config

# Fedora / RHEL
sudo dnf install -y gtk3-devel webkit2gtk4.1-devel
```

#### Building Headless Daemon
```bash
go build -o bin/sshnatd ./cmd/sshnatd
```
Run all configured tunnels:
```bash
./bin/sshnatd -config ./config.json -all
```

---

### 📁 Portable Mode & Config Layout
- **Portable Priority**: When the directory containing the binary is writable, `config.json` and `known_hosts` are kept directly beside the executable.
- **Safety Fallback**: When placed in read-only directories (e.g. Linux `/usr/bin/` or macOS `.app/Contents/MacOS/`), configuration falls back automatically to:
  - Windows: `%APPDATA%\sshnat\config.json`
  - Linux: `~/.config/sshnat/config.json`
  - macOS: `~/Library/Application Support/sshnat/config.json`
- **Path Expansion**: Private key paths support `~` (e.g. `~/.ssh/id_ed25519`), which expands seamlessly across Windows, Linux, and macOS.

---

### 🖥️ System Tray Capabilities
- **Background Daemon**: Closing the window hides it into the tray, ensuring active forwarders remain uninterrupted.
- **Quick Switcher**: The tray menu dynamically presents the most recently used tunnels as interactive checkboxes, enabling one-click background toggling.
- **Global Actions**: Start All, Stop All, Open Config Folder, and Quit.

---

### 🧪 Testing & Quality Gate
```bash
# Run all Go core and end-to-end tests (22 test cases)
go test -v ./...

# Run frontend Vitest suite (12 test cases)
cd frontend && npm test
```

---

### 📄 License
This project is open-source software licensed under the [MIT License](LICENSE).
