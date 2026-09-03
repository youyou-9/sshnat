//go:build !windows

// Package agentdial 提供 ssh-agent 的跨平台拨号。
// Unix-like（Linux/macOS）走 Unix socket。
package agentdial

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Dial 连接 ssh-agent。socket 为空时使用 SSH_AUTH_SOCK。
func Dial(socket string) (net.Conn, error) {
	if socket == "" {
		socket = DefaultPath()
	}
	return net.DialTimeout("unix", socket, 5*time.Second)
}

// DefaultPath 返回默认的 agent socket 路径。
func DefaultPath() string {
	if v := os.Getenv("SSH_AUTH_SOCK"); v != "" {
		return v
	}
	if runtime.GOOS == "darwin" {
		matches, err := filepath.Glob("/private/tmp/com.apple.launchd.*/Listeners")
		if err == nil && len(matches) > 0 {
			return matches[0]
		}
		return "/tmp/agent.sock"
	}
	return "/tmp/agent.sock"
}
