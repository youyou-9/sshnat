# SSHNat 发行候选审计

更新：2026-10-05。基线提交：`4576098`。

## 已修复和新增

- SSH 生命周期：连接取消、停止超时保护、远端请求阻塞、禁用保活后的断线检测、跳板和 agent 资源释放。
- 转发：双向半关闭、准确流量计数、Unix socket 所有权保护、IPv6、TCP/socket 混合路由。
- 配置：Windows 原子替换、严格 JSON/schema 检查、CRUD 与替换导入互斥、主机引用和跳板环路校验。
- 操作界面：批量操作错误、每隧道忙碌状态、命令复制反馈、长路由和最小窗口表格操作列。
- 新选项：主机连接测试、有序 ProxyJump、连接超时、主机密钥策略、known_hosts、保活、本地/SOCKS 绑定地址。
- 命令互通：POSIX/PowerShell 明确选择和解析，处理空格、引号、Windows 路径及 IPv6/socket。
- 配置迁移：默认脱敏导出、可选完整备份、文件读取与粘贴、合并和替换、替换前原文件备份及恢复。
- 发行：动态版本、Windows 原生版本信息、独立 daemon Docker 镜像、六目标交叉构建、包内文档及校验和。

## 本机验证结果

| 检查 | 结果 |
| --- | --- |
| Go 全包测试、vet、code-gate 安全检查 | 通过 |
| Svelte/TypeScript | 0 错误、0 警告 |
| 前端单元及 DOM 测试 | 6 个文件、35 项通过 |
| 前端生产构建、npm audit | 通过；0 漏洞 |
| Windows 完整桌面构建 | 通过；原生文件/产品版本 1.0.0，产品名 SSHNat |
| Windows/Linux/macOS × amd64/arm64 daemon 构建 | 六目标通过；Windows `-version` 输出 SSHNat 1.0.0 |
| Wails server 实际 RPC、事件及 UI | 使用独立临时配置验证，未使用用户配置 |
| 主机保存、连接测试 | 本机真实 SSH 握手成功 |
| UI 创建的本地转发与 SOCKS5 | 各通过 256 KiB 随机数据回显及半关闭；UI 累计流量正确 |
| 启停清理 | 全部停止后监听端口已释放 |
| 配置导出/导入 | 脱敏、非法版本拒绝、IPv6/socket 合并、运行时替换拒绝、停止后备份替换与文件恢复通过 |
| 外观 | 中文浅色、英文深色；960×600 表格操作按钮可见，无页面横向溢出 |
| 浏览器错误 | 最新生产构建 0 console errors；保留 Wails 浏览器模式提示 |
| actionlint、git diff --check | 通过 |

界面测试使用 Wails 的 server 构建连接真实 Go 服务，以及仅绑定 loopback 的临时
SSH/echo 服务。该测试验证 RPC、状态事件、配置与转发，未覆盖原生托盘和文件对话框。
临时程序、配置和截图位于被忽略的 `bin/`、`output/playwright/`，不进入发行包。

## 发行门禁

完整 CI 须通过 Linux race、Docker smoke、三平台 GUI、六目标 daemon，以及九个
发行压缩包的解压、内容、执行权限和 SHA256 检查。PR 会生成 `dev` 预览产物；
只有最终 `v*` tag 才发布 GitHub Release。

发行前还需按 [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) 确认原生托盘、窗口关闭行为、
原生保存对话框，以及目标平台的实际运行。当前本机 Docker engine 未运行，race
所需 C 编译器不可用；这些检查由 CI 执行。Linux deb/rpm/arch 安装包不属于当前
发行产物；GTK3 安装包需使用匹配的 NFPM 依赖并进行实际安装验证。

本报告记录已执行的检查，不表示任意环境或未覆盖场景不存在缺陷。
