# SSHNat

<div align="center">

**现代轻量级 SSH 端口转发客户端与无头后台守护进程**  
*A Modern Lightweight SSH Port Forwarding Desktop Client & Headless Daemon*

[![Build Status](https://github.com/youyou-9/sshnat/actions/workflows/build.yml/badge.svg)](https://github.com/youyou-9/sshnat/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26.6+-00ADD8?logo=go)]()
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
- [🔐 主机高级设置](#-主机高级设置)
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
- **OpenSSH 命令导入与复制**：
  - 支持直接粘贴标准 OpenSSH 命令行（`ssh -L ... -i ... -p ... user@host`）一键解析导入。
  - 根据当前隧道与主机设置生成带正确引号且可一键复制的 OpenSSH 命令。
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
- **Go**：1.26.6+（包含发行检查要求的安全修复）
  - 可选的混淆构建需从 [go.dev/dl](https://go.dev/dl/) 直接安装 Go 1.26.6；Garble 无法修补 Go 模块缓存中自动下载的工具链。
- **Node.js**：24.15+ 与 npm（锁定的 Vite/Vitest/jsdom 工具链要求 Node 24.15+）
- **Wails 3 CLI**：
  ```bash
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.11
  ```

#### 构建桌面客户端
```bash
# 1. 克隆代码仓库
git clone https://github.com/youyou-9/sshnat.git
cd sshnat

# 2. 安装前端依赖
cd frontend && npm ci && cd ..

# 3. 本地开发实时预览
wails3 task dev

# 4. 生产环境打包构建
wails3 task build
```
构建产物生成在 `bin/` 目录下（Windows 下为 `bin/sshnat.exe`，支持纯绿色直接拷贝运行）。

*注：Linux 系统若使用系统渲染器需安装开发依赖：*
```bash
# Debian 13+ / Ubuntu 24.04+（默认 GTK4 渲染器）
sudo apt install -y build-essential libgtk-4-dev libwebkitgtk-6.0-dev pkg-config

# Fedora / RHEL
sudo dnf install -y gtk4-devel webkitgtk6.0-devel
```
默认渲染器要求 GTK 4.14+。如发行版仅提供 GTK3/WebKitGTK 4.1，可安装对应开发包并使用
`wails3 task build EXTRA_TAGS=gtk3`（Wails 3.0 的兼容渲染器）。
创建 Linux 安装包时也应使用相同的 `EXTRA_TAGS=gtk3`，例如
`wails3 task linux:create:deb EXTRA_TAGS=gtk3 VERSION=1.0.0`。
deb/rpm/Arch 打包任务会自动选择匹配的运行时依赖，并保留二进制的执行权限。

#### 构建无头守护进程
适用于无界面服务器或容器环境：
```bash
# 本地构建在关于页和窗口标题中显示 dev
go build -o bin/sshnatd ./cmd/sshnatd

# 使用 Taskfile 构建全部发行版目标（需要 Taskfile）
wails3 task daemon:all VERSION=1.0.0
```
快速启动所有配置隧道：
```bash
./bin/sshnatd -config ./config.json -all
```
无需读取配置文件即可检查 daemon 版本：
```bash
./bin/sshnatd -version
```
无需联网或启动隧道即可校验配置：
```bash
./bin/sshnatd -config ./config.json -check
```
文件缺失、未知字段、主机或隧道参数无效、引用缺失及 ProxyJump 环路都会使 `-check` 非零退出。
校验模式接受合法的空配置；正常启动需要至少一条被选中的隧道。

构建最小容器镜像（仅包含 `sshnatd`，不包含 GUI 或 HTTP 监听器）：
```bash
wails3 task common:build:docker TAG=sshnat:1.0.0 VERSION=1.0.0
docker run --rm \
  --mount "type=bind,source=$PWD/sshnat-config,target=/config" \
  sshnat:1.0.0 -config /config/config.json -all
```

将 `config.json` 放在挂载目录中；程序学习到的 `known_hosts` 也会持久保存在该目录。
配置中的私钥路径应指向容器内可访问的文件。如需通过 Docker 端口映射访问本地 TCP/SOCKS
监听器，请将隧道的 `localBindHost` 设为 `0.0.0.0`，并添加对应的 `-p 宿主端口:容器端口`。
配置不存在或隧道选择无效时 daemon 以非零状态退出；容器生命周期反映进程是否退出，目标服务
可用性应通过实际转发服务进行监控。

发行版构建会从 `v*` Git tag 注入版本号；本地需要可重复版本时，可向 Taskfile 命令传入
`VERSION=1.0.0`。桌面构建还会同步 Windows 文件信息/MSIX/NSIS、macOS plist、Linux NFPM 的
数字版本号；预发行后缀保留在应用和 daemon 的版本输出中。

---

### 📁 便携模式与配置规范
SSHNat 支持零侵入的便携模式：
- **便携策略**：当程序所在目录可写时，优先读写程序同目录下的 `config.json` 与 `known_hosts`。
- **安全回退**：当运行在只读路径（如 Linux `/usr/bin/` 或 macOS `.app/Contents/MacOS/`）时，自动回退至用户配置目录：
  - Windows: `%APPDATA%\sshnat\config.json`
  - Linux: `~/.config/sshnat/config.json`
  - macOS: `~/Library/Application Support/sshnat/config.json`
- **路径展开**：私钥与已知主机文件路径支持波浪号 `~`（如 `~/.ssh/id_rsa`），程序会跨平台解析为当前用户目录。

---

### 🔐 主机高级设置

打开 **主机 → 添加/编辑 → 高级设置** 可调整：

| 设置 | 行为 |
| --- | --- |
| 连接超时 | 1–300 秒，默认 15 秒；JSON 中 `connectTimeoutSeconds: 0` 同样表示默认值。 |
| Keepalive 间隔 | `0` 使用默认 15 秒，`-1` 关闭探测，`1`–`86400` 设置明确的秒数。 |
| 主机密钥策略 | `accept-new` 记录未知主机密钥；`strict` 要求已有匹配记录。两者都拒绝已知主机的密钥变更。 |
| 已知主机文件 | 留空使用当前 `config.json` 同目录的 `known_hosts`；可指定已有可信 OpenSSH 文件。严格模式不会创建文件。 |
| 跳板链 | 添加、移除或调整已保存主机的顺序，从最近到最远排列。每一跳保留各自的认证、主机密钥策略及超时，保存时拒绝环路。 |

省略 `hostKeyPolicy` 的新旧配置均默认使用 `accept-new`。使用 `strict` 前，需将经过核实的
服务器公钥放入指定的已知主机文件。测试失败会显示握手或主机密钥错误，并保留当前策略。

复制的命令通过 `-o` 保留目标主机的密钥策略、连接超时、Keepalive、明确指定的已知主机文件和
Agent socket。导入时选项名不区分大小写，支持 `Name=value` 和带引号的 `Name value`。
`ServerAliveInterval=0` 对应应用中关闭 Keepalive 的 `-1`；应用默认值 `0` 或省略时，
复制为 15 秒。导入会拒绝 `StrictHostKeyChecking=no/off/ask`、无期限的 `ConnectTimeout=0`、
多个已知主机文件以及 `-i` 与 `IdentityAgent` 混用，因为应用无法表达这些设置。

应用中已知主机路径留空时使用当前 `config.json` 同目录的 `known_hosts`，OpenSSH 则使用
自己的默认 `~/.ssh/known_hosts`。要让复制的命令复用应用中已信任的指纹，请在主机设置中
明确填写同一个文件路径。`-J` 只能携带跳板地址、用户和端口，无法携带每个跳板独立的密钥、
Agent socket、密钥策略或超时；运行复制的多跳命令前，需在 OpenSSH config 中配置各跳板。
命令导入不会读取 `-F` 指定的 SSH config 文件。迁移全部应用设置和已保存凭据时，请使用完整配置备份。

---

### 配置迁移与备份

设置页支持保存或复制配置 JSON，也可读取文件或粘贴 JSON 导入。默认导出
不含密码和私钥口令；需要迁移凭据时可启用**完整备份**并保存至私有位置。
私钥文件与 `known_hosts` 需另行复制。

- **合并**：保留原有条目，为导入主机和隧道分配新 ID，并保持跳板引用。
- **替换**：须先停止全部隧道，原配置会完整备份为配置目录下的
  `config-backup-*.json`；再次导入该文件即可恢复。
- 导入的隧道保持停止状态，自动启动设置在下次打开应用时生效。
- 导入前拒绝未知字段、不支持的版本、无效引用和跳板环路；无版本的旧文档
  按版本 1 读取。

### 🖥️ 系统托盘操作说明
1. **最小化到托盘**：点击窗口右上角关闭按钮（X），应用自动隐藏至系统托盘，后台所有隧道保持畅通连接。
2. **免开窗口快捷管理**：右键托盘图标，菜单列出最近使用的高频隧道，直接点击勾选框即可静默启停该隧道。
3. **全局控制**：托盘提供“全部启动”、“全部停止”、“打开配置目录”及“彻底退出”操作。

---

### 🧪 测试与质量门禁
项目包含严苛的自动化测试套件：
```bash
# 执行 Go 核心与网络端到端测试
go test -v ./...

# 执行前端组件、状态与国际化单测
cd frontend && npm test
```

发行前请执行 [发行检查清单](RELEASE_CHECKLIST.md) 中的构建、运行与打包门禁。

---

### 📄 开源协议
本项目基于 [MIT License](LICENSE) 协议开源。
