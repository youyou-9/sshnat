package main

import (
	"fmt"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/supervisor"
)

// trayBI 按界面语言选择托盘文案；lang 只取 "zh" | "en"，缺省中文。
func trayBI(lang, zh, en string) string {
	if lang == "en" {
		return en
	}
	return zh
}

func trayTunnelLabel(tunnel config.Tunnel, status supervisor.Status, lang string) string {
	statusText := trayBI(lang, "已停止", "Stopped")
	switch status {
	case supervisor.StatusStarting:
		statusText = trayBI(lang, "正在连接", "Connecting")
	case supervisor.StatusConnected:
		statusText = trayBI(lang, "运行中", "Running")
	case supervisor.StatusReconnecting:
		statusText = trayBI(lang, "重连中", "Reconnecting")
	case supervisor.StatusStopping:
		statusText = trayBI(lang, "停止中", "Stopping")
	case supervisor.StatusError:
		statusText = trayBI(lang, "错误", "Error")
	}
	endpoint := fmt.Sprint(tunnel.LocalPort)
	if tunnel.LocalSocket != "" {
		endpoint = tunnel.LocalSocket
	}
	switch tunnel.Type {
	case config.TypeDynamic:
		endpoint = fmt.Sprint(tunnel.SocksPort)
	case config.TypeRemote:
		endpoint = fmt.Sprint(tunnel.RemotePort)
		if tunnel.RemoteSocket != "" {
			endpoint = tunnel.RemoteSocket
		}
	}
	return fmt.Sprintf("[%s] %s (-%s: %s)", statusText, tunnel.Name, tunnel.Type, endpoint)
}

// A menu may remain open while a tunnel changes state. Read its lifecycle at
// click time so a reconnecting or stopping run is stopped instead of restarted.
func toggleTrayTunnel(id string, active func(string) bool, start, stop func(string) error) error {
	if active(id) {
		return stop(id)
	}
	return start(id)
}
