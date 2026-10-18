//go:build windows

package autostart

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func platformIsEnabled(exe string) bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetStringValue(appID)
	if err != nil {
		return false
	}
	return value == quotedPath(exe)
}

func platformEnable(exe string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run registry key: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue(appID, quotedPath(exe)); err != nil {
		return fmt.Errorf("write Run registry value: %w", err)
	}
	return nil
}

func platformDisable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return nil // 键不存在即未注册
	}
	defer key.Close()
	if err := key.DeleteValue(appID); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete Run registry value: %w", err)
	}
	return nil
}

// quotedPath 加引号保护含空格的路径（Run 键按命令行解析）。
func quotedPath(exe string) string {
	return `"` + exe + `"`
}
