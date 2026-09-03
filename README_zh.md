# SSHNat

<div align="center">

**现代轻量级 SSH 端口转发客户端与无头后台守护进程**  
*A Modern Lightweight SSH Port Forwarding Desktop Client & Headless Daemon*

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)]()
[![Wails Version](https://img.shields.io/badge/Wails-v3.0.0--beta.11-DF0000?logo=wails)]()
[![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte)]()
[![License](https://img.shields.io/badge/License-MIT-blue.svg)]()

[🇬🇧 English Documentation](README.md) · [🇨🇳 简体中文](README_zh.md)

</div>

---

## 目录
- [📖 项目简介](#-项目简介)
- [✨ 核心功能](#-核心功能)
- [🏗️ 项目结构](#-项目结构)
- [🚀 快速开始与构建](#-快速开始与构建)
  - [系统环境依赖](#系统环境依赖)
  - [构建桌面客户端](#构建桌面客户端)
  - [构建无头守护进程](#构建无头守护进程)
- [📁 便携模式与配置规范](#-便携模式与配置规范)
- [🖥️ 系统托盘操作说明](#-系统托盘操作说明)
- [🧪 测试与质量门禁](#-测试与质量门禁)
- [📄 开源协议](#-开源协议)

---

### 📖 项目简介
**SSHNat** 是一款现代、高效且深度解耦的 SSH 端口转发管理工具。它结合了 Go 语言原生并发网络引擎的高性能与 Svelte 5 + Tailwind CSS v4 带来的现代桌面交互体验（基于 Wails v3 运行时）。同时提供了独立的无头 CLI 守护进程（`sshnatd`），满足从个人开发、日常运维到服务器无界面常驻的各类场景。

---

### ✨ 核心功能
- **全模式端口转发**：
  - **本地转发 (`-L`)**：本地监听端口经 SSH 隧道访问远端私网服务或本地回环服务（支持 TCP 与 Unix Socket）。
  - **远程反向转发 (`-R`)**：在远端 SSH 服务器监听，接收连接并回连本机目标服务（内网穿透）。
  - **动态代理 (`-D`)**：本地提供开箱即用的高并发 SOCKS5 代理。
- **全场景身份认证**：
  - 支持账号密码、加密私钥口令（Passphrase）、系统与第三方 SSH Agent（包括 Windows 命名管道 `\\.\pipe\openssh-ssh-agent` / `1password-ssh-agent` 及 Unix Socket）。
  - 支持多级跳板机链（`ProxyJump`）与环路检测。
- **高韧性生命周期管理**：
  - 原生支持 OpenSSH Keepalive 探测与指数退避（Exponential Backoff）自愈自动重连。
  - 双向数据流协同关闭，杜绝半开连接（Half-Close）与孤儿套接字泄漏。
- **双向 OpenSSH CLI 命令互通**：
  - 支持直接粘贴标准 OpenSSH 命令行（`ssh -L ... -i ... -p ... user@host`）一键解析导入。
  - 自动根据当前隧道与主机凭据生成规范且可一键复制的完整 OpenSSH 命令。
- **实时指标与精细流式排错**：
  - 双向瞬时速率、累计吞吐量及 60 点 Sparkline 实时折线图。
  - 详细的连接握手、目标拨号与拒绝原因（如 Connection Refused、Timeout）实时流式日志。
  - 专属每条隧道的独立日志查看器（无需切换主屏即可排错）。
- **后台常驻与动态托盘**：
  - 窗口关闭隐藏至系统托盘，保持隧道平稳常驻。
  - 托盘右键菜单**动态列出最近使用的隧道**（按使用时间倒序排列，超额优雅折叠），无需弹出主界面即可直接勾选启停。
- **便携模式（Portable Mode）**：
  - 自动优先使用与可执行文件同级的 `config.json`，支持打包即走；在只读目录中自动安全回退至系统标准配置目录。
- **双语国际化与主题外观**：
  - 原生集成深色 / 浅色 / 跟随系统主题，完美支持中英文（`zh` / `en`）无缝切换。

---

### 🏗️ 项目结构

```text
sshnat/
├── app/                  # Wails v3 应用层：服务绑定、参数校验、事件中继与命令行解析
├── build/                # 全平台打包元数据（macOS .app、Linux NFPM、Windows 图标/清单、Docker）
├── cmd/
│   └── sshnatd/          # 无头（Headless）后台 CLI 守护进程入口
├── core/                 # 核心网络引擎（纯 Go 编写，零 GUI 依赖）
│   ├── config/           # 配置数据模型、CLI 解析器、原子持久化与便携模式解析
│   ├── forward/          # 端口转发器（local.go, remote.go, dynamic.go, 流量统计与协程泵）
│   ├── internal/         # 跨平台 SSH-Agent 命名管道 / 套接字驱动
│   ├── ssh/              # SSH 客户端生命周期、多跳 ProxyJump、密码/密钥认证与已知主机密钥管理
│   ├── stats/            # 并发安全的每隧道原子流量与连接计数器
│   └── supervisor/       # 隧道管理状态机、断线退避重连、统计定时分发与事件总线
├── frontend/             # 前端项目（Svelte 5 Runes + Vite + Tailwind CSS v4）
│   ├── bindings/         # Wails 自动生成的 TypeScript 绑定代码
│   └── src/              # 前端源码（Dashboard 监控大盘, Tunnels 控制中心, Logs, Hosts, Settings）
├── main.go               # 桌面客户端入口（Wails v3 应用组装与托盘控制）
├── go.mod / go.sum       # Go 依赖定义
├── README.md             # 英文说明文档（默认）
├── README_zh.md          # 中文说明文档
└── Taskfile.yml          # 自动化构建与打包任务清单
```

---

### 🚀 快速开始与构建

#### 系统环境依赖
- **Go**：1.25+
- **Node.js**：18+ 与 npm
- **Wails 3 CLI**：
  ```bash
  go install github.com/wailsapp/wails/v3/cmd/wails3@latest
  ```

#### 构建桌面客户端
```bash
# 1. 克隆代码仓库
git clone https://github.com/youyou-9/sshnat.git
cd sshnat

# 2. 安装前端依赖
cd frontend && npm install && cd ..

# 3. 本地开发实时预览
wails3 task dev

# 4. 生产环境打包构建
wails3 task build
```
构建产物生成在 `bin/` 目录下（Windows 下为 `bin/sshnat.exe`，支持纯绿色直接拷贝运行）。

*注：Linux 系统若使用系统渲染器需安装开发依赖：*
```bash
# Debian / Ubuntu
sudo apt install -y build-essential libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config

# Fedora / RHEL
sudo dnf install -y gtk3-devel webkit2gtk4.1-devel
```

#### 构建无头守护进程
适用于无界面服务器或容器环境：
```bash
go build -o bin/sshnatd ./cmd/sshnatd
```
快速启动所有配置隧道：
```bash
./bin/sshnatd -config ./config.json -all
```

---

### 📁 便携模式与配置规范
SSHNat 支持零侵入的便携模式：
- **便携策略**：当程序所在目录可写时，优先读写程序同目录下的 `config.json` 与 `known_hosts`。
- **安全回退**：当运行在只读路径（如 Linux `/usr/bin/` 或 macOS `.app/Contents/MacOS/`）时，自动回退至用户配置目录：
  - Windows: `%APPDATA%\sshnat\config.json`
  - Linux: `~/.config/sshnat/config.json`
  - macOS: `~/Library/Application Support/sshnat/config.json`
- **私钥路径展开**：私钥路径支持通用波浪号 `~`（如 `~/.ssh/id_rsa`），程序会自动跨平台解析为主机真实用户目录。

---

### 🖥️ 系统托盘操作说明
1. **最小化到托盘**：点击窗口右上角关闭按钮（X），应用自动隐藏至系统托盘，后台所有隧道保持畅通连接。
2. **免开窗口快捷管理**：右键托盘图标，菜单列出最近使用的高频隧道，直接点击勾选框即可静默启停该隧道。
3. **全局控制**：托盘提供“全部启动”、“全部停止”、“打开配置目录”及“彻底退出”操作。

---

### 🧪 测试与质量门禁
项目包含严苛的自动化测试套件：
```bash
# 执行 Go 核心与网络端到端测试（22 个用例）
go test -v ./...

# 执行前端组件、状态与国际化单测（12 个用例）
cd frontend && npm test
```

---

### 📄 开源协议
本项目基于 [MIT License](LICENSE) 协议开源。
