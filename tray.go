package main

import (
	"fmt"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/supervisor"
)

func trayTunnelLabel(tunnel config.Tunnel, status supervisor.Status) string {
	statusText := "已停止"
	switch status {
	case supervisor.StatusStarting:
		statusText = "正在连接"
	case supervisor.StatusConnected:
		statusText = "运行中"
	case supervisor.StatusReconnecting:
		statusText = "重连中"
	case supervisor.StatusStopping:
		statusText = "停止中"
	case supervisor.StatusError:
		statusText = "错误"
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
