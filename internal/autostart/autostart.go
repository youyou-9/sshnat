// Package autostart 管理“开机自启动”：把当前可执行文件注册到
// 各平台标准的登录自启动位置（Windows 注册表 Run 键、
// macOS LaunchAgents、Linux XDG autostart）。
package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

// appID 是各平台自启动条目使用的稳定标识。
const appID = "SSHNat"

// IsEnabled 报告当前是否已注册开机自启动。
func IsEnabled() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return platformIsEnabled(exe)
}

// Enable 注册开机自启动；重复调用幂等（更新为最新路径）。
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	return platformEnable(exe)
}

// Disable 取消开机自启动；未注册时视为成功。
func Disable() error {
	return platformDisable()
}
