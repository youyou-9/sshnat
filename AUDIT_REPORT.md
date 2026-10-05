# SSHNat 发行候选审计

更新：2026-10-05。基线提交：`4576098`。

## 已修复和新增

- SSH 生命周期：连接取消、停止超时保护、远端请求阻塞、禁用保活后的断线检测、跳板和 agent 资源释放。
- 转发：双向半关闭、准确流量计数、Unix socket 所有权保护、IPv6、TCP/socket 混合路由。
- 配置：Windows 原子替换、严格 JSON/schema 检查、CRUD 与替换导入互斥、主机引用和跳板环路校验。
- 操作界面：批量操作错误、每隧道忙碌状态、命令复制反馈、长路由和最小窗口表格操作列。
- 新选项：主机连接测试、有序 ProxyJump、连接超时、主机密钥策略、known_hosts、保活、本地/SOCKS 绑定地址。
- 命令互通：POSIX/PowerShell 明确选择和解析，处理空格、引号、Windows 路径及 IPv6/socket。
- 高级命令往返：保留主机密钥策略、超时、保活和 agent；复用主机时仅覆盖显式选项，同私钥保留口令、更换私钥清除旧口令。
- 配置迁移：默认脱敏导出、可选完整备份、文件读取与粘贴、合并和替换、替换前原文件备份及恢复。
- 发行：动态版本、Windows 原生版本信息、独立 daemon Docker 镜像、六目标交叉构建、包内文档及校验和。
- 原生界面：配置变更刷新托盘，重连中可停止；UI 线程更新与退出互斥，退出后禁止排队启动；替换 Wails 默认图标。
- 安全：Go 1.26.6、x/crypto v0.56.0 修复本次扫描发现的可达漏洞；CI 增加 npm audit 与 govulncheck。

## 本机验证结果

| 检查 | 结果 |
| --- | --- |
| Go 全包测试、vet、code-gate 安全检查 | 通过 |
| Svelte/TypeScript | 0 错误、0 警告 |
| 前端单元及 DOM 测试 | 6 个文件、42 项通过 |
| 前端生产构建、npm audit | 通过；0 漏洞 |
| Windows 完整桌面构建 | 候选 1.1.0-rc.1 通过；原生文件/产品数字版本 1.1.0，产品名 SSHNat |
| Windows/Linux/macOS × amd64/arm64 daemon 构建 | 六目标通过；Windows `-version` 输出 SSHNat 1.1.0-rc.1 |
| Wails server 实际 RPC、事件及 UI | 使用独立临时配置验证，未使用用户配置 |
| 主机保存、连接测试 | 本机真实 SSH 握手成功 |
| UI 创建的本地转发与 SOCKS5 | 各通过 256 KiB 随机数据回显及半关闭；UI 累计流量正确 |
| 启停清理 | 全部停止后监听端口已释放 |
| 配置导出/导入 | 脱敏、非法版本拒绝、IPv6/socket 合并、运行时替换拒绝、停止后备份替换与文件恢复通过 |
| 外观 | 中文浅色、英文深色；960×600 表格操作按钮可见，无页面横向溢出 |
| 浏览器错误 | Wails server 实际操作场景 0 console errors；保留 Wails 浏览器模式提示；最终原生构建另行验证 |
| actionlint、git diff --check | 通过 |
| Go 漏洞扫描 | 0 可达、0 导入包漏洞；未使用的 OpenPGP 模块提示见 DEPENDENCY_AUDIT.md |
| SSH 命令兼容 | 共享 fixture 经 TS formatter、shell 解析、Go 导入和保存往返；PowerShell/Git Bash 的实际 ssh -G 通过 |
| Windows 原生实际运行 | SSH 隧道 256 KiB 精确回显；关闭窗口后再次传输成功；再次启动恢复同一窗口；停止后端口释放 |
| Windows 原生文件保存 | 脱敏 JSON 保存成功（2 hosts / 3 tunnels）；取消返回错误的问题已修正、增加回归，并在重建后原生复测通过 |
| Linux 安装包元数据 | deb/rpm/Arch 架构、GTK3/GTK4 选择与 0755 权限验证；本机仅使用临时 payload 检查元数据 |

界面测试使用 Wails 的 server 构建连接真实 Go 服务，以及仅绑定 loopback 的临时
SSH/echo 服务。该测试验证 RPC、状态事件、配置与转发；另以 Windows 原生 WebView2
实测窗口关闭/恢复、文件保存、启停及传输。托盘的真实点击和 macOS 原生退出仍需手测。
临时程序、配置和截图位于被忽略的 `bin/`、`output/playwright/`，不进入发行包。

## 发行门禁

完整 CI 须通过 Linux race、Docker smoke、三平台 GUI、六目标 daemon，以及九个
发行压缩包的解压、内容、执行权限和 SHA256 检查。PR 会生成 `dev` 预览产物；
只有最终 `v*` tag 才发布 GitHub Release。

最终代码提交 `6f8c1c7b664f2c58cccef1f25a8b1d2a3f182d78` 的完整 CI 已通过：
[run 37236286787](https://github.com/youyou-9/sshnat/actions/runs/37236286787)。
质量门禁（含 race 和漏洞扫描）、三平台 GUI、六目标 daemon、Docker 及发行包
校验全部成功；Linux GUI job 也校验了真实 GTK3 deb，并在 Xvfb 中通过原生启动检查。
`release-files` artifact 包含九个已验证的发行预览压缩包和 `SHA256SUMS`。

发行前还需按 [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) 确认原生托盘及 macOS
实际关闭/退出/保存行为。当前本机 Docker engine 未运行，race 所需 C 编译器不可用；
这些检查由 CI 执行。Linux deb/rpm/Arch 安装包不属于当前发行产物；虽然依赖选择
与权限已修复，其目标发行版安装和桌面集成仍需实际验证。

本机 Node 25.9 不在 jsdom 30 的声明支持范围内，安装有 engine 提示但测试和构建通过；
CI 使用受支持的 Node 24.15 再次验证。现有正式版本是 v1.0.0，新增功能以
1.1.0-rc.1 作本机候选验证。GitHub `main` 构建只生成预览 artifact，只有推送
`v*` tag 才会执行 Release job；包含连字符的版本（例如 `v1.1.0-rc.1`）会标记为
GitHub prerelease。

## 发行评估

本轮已发现的问题均已修复，代码及自动化发行门禁通过，可作为 **1.1.0-rc.1
发行候选版**交付验证。Windows 本机候选程序使用该版本；PR 的跨平台预览包使用
`dev` 版本，不能直接视为最终 tag 的产物。

稳定版仍需真实托盘菜单点击/退出、macOS 原生关闭/退出/保存检查。支持的本机
自动化工具仅暴露应用窗口，未完成 Windows 系统托盘的实际点击；单元测试和
构建成功不能替代这项验证。Linux 安装包的目标发行版安装验证也尚未执行，且这些
安装包不属于当前九个发行压缩包。不将上述未验证范围宣称为已通过。

修复及功能改进已合并自 [PR #2](https://github.com/youyou-9/sshnat/pull/2)。
候选发布命令为 `git tag -a v1.1.0-rc.1 -m "Release SSHNat v1.1.0-rc.1"`
和 `git push origin v1.1.0-rc.1`；tag workflow 会重新执行全部门禁并上传已校验的包。

本报告记录已执行的检查，不表示任意环境或未覆盖场景不存在缺陷。
