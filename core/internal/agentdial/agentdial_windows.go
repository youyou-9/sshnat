//go:build windows

// Package agentdial 的 Windows 实现：
// 支持 OpenSSH for Windows 默认命名管道 \\.\pipe\openssh-ssh-agent。
package agentdial

import (
	"net"
	"os"
	"time"

	winio "github.com/Microsoft/go-winio"
)

// Dial 连接 ssh-agent。socket 为空时使用默认命名管道。
func Dial(socket string) (net.Conn, error) {
	if socket == "" {
		socket = DefaultPath()
	}
	timeout := 5 * time.Second
	return winio.DialPipe(socket, &timeout)
}

// DefaultPath 返回 Windows 默认的 agent 命名管道。
func DefaultPath() string {
	if v := os.Getenv("SSH_AUTH_SOCK"); v != "" {
		return v
	}
	return `\\.\pipe\openssh-ssh-agent`
}
