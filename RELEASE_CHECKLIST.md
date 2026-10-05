# SSHNat 发行检查清单

## 1. 自动化质量门禁

在仓库根目录执行（Go 1.26.6+、Node.js 24.15+）：

```bash
npm --prefix frontend ci
npm --prefix frontend audit
npm --prefix frontend run check
npm --prefix frontend test -- --run
npm --prefix frontend run build
go test ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
git diff --check
```

Linux 使用 GTK3 开发包时，Go 命令需添加 `-tags gtk3`。安装 C 编译器后执行
`go test -race -tags gtk3 ./...`；CI 的 Linux quality job 包含该门禁。

## 2. 版本及交叉编译

将 `1.0.0` 替换为本次发行版本，版本应与最终 `v*` tag 一致：

```bash
go run ./cmd/sshnat-buildmeta -version 1.0.0
wails3 task daemon:all VERSION=1.0.0
wails3 task common:build:server VERSION=1.0.0
```

Windows 使用 `bin/sshnatd.exe -version`，Linux/macOS 使用
`./bin/sshnatd -version`。输出应为 `SSHNat 1.0.0`。确认 `bin/` 包含
Windows/Linux/macOS × amd64/arm64 六个 daemon 产物，Windows 后缀为 `.exe`。

daemon 构建依赖中不应包含桌面运行时：

```bash
go list -deps ./cmd/sshnatd
```

输出中不应出现 `github.com/wailsapp/wails`。

## 3. 桌面实际使用

在每个平台构建并启动：

```bash
wails3 task build VERSION=1.0.0
```

Linux GTK3 构建添加 `EXTRA_TAGS=gtk3`；macOS 用
`wails3 task darwin:package VERSION=1.0.0` 创建 `.app`。

Linux deb/rpm/Arch 安装包需在构建和打包时使用相同的 `EXTRA_TAGS`。
例如 `wails3 task linux:create:deb EXTRA_TAGS=gtk3 VERSION=1.0.0` 会自动
选用 GTK3/WebKitGTK 4.1 依赖。CI 校验 GTK3 deb 的架构、依赖和执行权限；
安装包在目标发行版中的实际安装及桌面集成仍需单独验证。

检查以下行为：

- 窗口标题、侧栏版本、设置关于页及原生文件/包信息使用同一个发行版本。
- 新建、编辑、删除主机与隧道；每种认证方式及跳板链可保存并再次读取。
- 主机高级设置中超时、Keepalive、已知主机文件及跳板顺序保存后不丢失；环路、非法范围被拒绝。
- `strict` 拒绝未知或变更密钥；`accept-new` 学习未知密钥后 `strict` 可连接，学习后换钥仍被拒绝。
- 严格模式不会创建缺失的已知主机文件；连接测试使用所配置的超时并显示失败原因。
- 本地、远程、SOCKS5、Unix Socket 转发可传输数据；IPv6 路由显示和命令复制正确。
- 连接失败时显示可操作错误；断网恢复后重连；启停和更新运行中隧道不留下占用端口。
- 明暗主题与中英文切换无缺字、溢出；窗口关闭后隧道保持，托盘退出后释放连接。

## 4. 容器实际运行

```bash
wails3 task common:build:docker TAG=sshnat:1.0.0 VERSION=1.0.0
docker run --rm sshnat:1.0.0 -version
docker run --rm --mount "type=bind,source=$PWD/sshnat-config,target=/config" sshnat:1.0.0 -config /config/config.json -check
docker run --rm sshnat:1.0.0 -config /missing/config.json -all
```

第一条运行命令输出 `SSHNat 1.0.0`；有效配置 `-check` 不联网并成功退出；缺少配置必须以非零状态退出。
CI 的 Docker job 在未构建桌面资源的 checkout 中执行镜像构建和这三项检查。

将真实 `config.json` 放在 `sshnat-config/` 中，使用 Bash 示例：

```bash
docker run --rm --name sshnat-test \
  --mount "type=bind,source=$PWD/sshnat-config,target=/config" \
  -p 8080:8080 sshnat:1.0.0 -config /config/config.json -all
```

PowerShell 使用 `${PWD}/sshnat-config` 表达挂载路径。容器内本地监听器需配置
`localBindHost: "0.0.0.0"`。确认服务可从宿主端口访问、`known_hosts` 持久保存，
并在另一终端执行 `docker stop sshnat-test` 后确认连接与容器退出。容器生命周期反映
daemon 是否退出；目标可用性使用实际转发服务探测。

## 5. 发行包门禁

- GitHub Build 的 quality、gui、daemon、docker、package jobs 全部通过。
- 每个压缩包包含二进制、`LICENSE`、中英文 README；Unix 解压后可直接执行。
- macOS `.app` 内 `Contents/MacOS/sshnat` 保留执行权限；Windows 包为 `.zip`。
- package job 生成 `SHA256SUMS`，release job 再次验证；下载后执行 `sha256sum -c SHA256SUMS` 验证文件。
- 最终 tag 与运行时/原生包版本一致，工作树已提交，发行说明列出用户可见的新功能和修复。

PR、`main` 推送与手动 workflow_dispatch 会生成并解压校验九个发行包，验证文档、Unix 执行权限及
`SHA256SUMS`，并上传 `release-files` artifact；非 tag 构建显示 `dev`。这些构建不会自动创建
GitHub Release。发布由 `v*` tag 触发，并下载同一份已经通过校验的产物；包含连字符的 tag（如
`v1.1.0-rc.1`）会标记为 prerelease：

```bash
git tag -a v1.1.0-rc.1 -m "Release SSHNat v1.1.0-rc.1"
git push origin v1.1.0-rc.1
```
