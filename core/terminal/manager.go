// Package terminal 管理交互式 SSH 远程终端会话：
// PTY + shell，输出经回调推送给 UI，输入/缩放/关闭由 UI 驱动。
// 本包为纯 Go 库，不依赖任何 UI 框架；事件通过注入的回调上报。
package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/sshnat/sshnat/core/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// DataCallback 在会话产生输出时触发（PTY 输出为原始字节流，可能不完整 UTF-8）。
type DataCallback func(sessionID string, data []byte)

// ExitCallback 在会话结束时触发；err 为空字符串表示正常退出。
type ExitCallback func(sessionID string, err string)

// Manager 跟踪全部打开的终端会话。
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*session

	onData DataCallback
	onExit ExitCallback
}

// session 是一个打开的终端会话。
type session struct {
	id     string
	client *ssh.Client
	stdin  io.WriteCloser
	resize func(cols, rows int) error
	wait   func() error
	gsess  *gossh.Session
	done   chan struct{}

	closeOnce    sync.Once
	closedByUser atomic.Bool
}

// shutdown 释放会话资源；由生命周期泵与 Close/CloseAll 共用，保证只执行一次，
// 且会同时关闭 *ssh.Session（此前主动关闭路径漏了它）。
func (s *session) shutdown() {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.stdin.Close()
		if s.gsess != nil {
			_ = s.gsess.Close()
		}
		_ = s.client.Close()
	})
}

// NewManager 创建 Manager；两个回调可为 nil（静默丢弃输出）。
func NewManager(onData DataCallback, onExit ExitCallback) *Manager {
	return &Manager{
		sessions: make(map[string]*session),
		onData:   onData,
		onExit:   onExit,
	}
}

var (
	// ErrNotFound 会话不存在或已关闭。
	ErrNotFound = errors.New("terminal session not found")
	// ErrClosing Manager 正在关闭，拒绝新会话。
	ErrClosing = errors.New("terminal manager is closing")
)

// Open 拨号并开启一个带 PTY 的交互 shell 会话。
// dialCtx 仅约束连接与握手；会话建立后不再受其超时影响。
func (m *Manager) Open(dialCtx context.Context, id string, opts ssh.DialOptions, cols, rows int) error {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	m.mu.Lock()
	if m.sessions == nil {
		m.mu.Unlock()
		return ErrClosing
	}
	if _, exists := m.sessions[id]; exists {
		m.mu.Unlock()
		return fmt.Errorf("terminal session %s already exists", id)
	}
	m.mu.Unlock()

	client, err := ssh.Dial(dialCtx, opts)
	if err != nil {
		return err
	}

	gsess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("open ssh session: %w", err)
	}

	stdin, err := gsess.StdinPipe()
	if err != nil {
		_ = gsess.Close()
		_ = client.Close()
		return fmt.Errorf("open session stdin: %w", err)
	}
	stdout, err := gsess.StdoutPipe()
	if err != nil {
		_ = gsess.Close()
		_ = client.Close()
		return fmt.Errorf("open session stdout: %w", err)
	}
	// 远端 stderr 合并进输出流，与常见终端行为一致。
	gsess.Stderr = gsess.Stdout

	modes := sshTerminalModes()
	if err := gsess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = gsess.Close()
		_ = client.Close()
		return fmt.Errorf("request pty: %w", err)
	}
	if err := gsess.Shell(); err != nil {
		_ = gsess.Close()
		_ = client.Close()
		return fmt.Errorf("start shell: %w", err)
	}

	s := &session{
		id:     id,
		client: client,
		stdin:  stdin,
		resize: func(c, r int) error { return gsess.WindowChange(r, c) },
		wait:   gsess.Wait,
		gsess:  gsess,
		done:   make(chan struct{}),
	}

	m.mu.Lock()
	if m.sessions == nil {
		m.mu.Unlock()
		_ = gsess.Close()
		_ = client.Close()
		return ErrClosing
	}
	// 拨号期间不持锁，可能已有同 id 会话写入；此处复查，避免覆盖并泄漏旧连接。
	if _, exists := m.sessions[id]; exists {
		m.mu.Unlock()
		_ = gsess.Close()
		_ = client.Close()
		return fmt.Errorf("terminal session %s already exists", id)
	}
	m.sessions[id] = s
	m.mu.Unlock()

	// 输出泵：PTY 输出转发给 UI（xterm.js 处理转义序列）。
	// 事件载荷为 JSON 字符串，因此只在完整 UTF-8 边界切分，
	// 避免多字节字符被一次 Read 切断后在前端变成乱码。
	go func() {
		buf := make([]byte, 32*1024)
		var pending []byte
		for {
			n, err := stdout.Read(buf)
			if n > 0 && m.onData != nil {
				pending = append(pending, buf[:n]...)
				if complete, rest := splitUTF8(pending); len(complete) > 0 {
					m.onData(id, complete)
					pending = rest
				}
			}
			if err != nil {
				if len(pending) > 0 && m.onData != nil {
					m.onData(id, pending) // 收尾：残缺字节也尽量送达
				}
				return
			}
		}
	}()

	// 生命周期泵：shell 退出或连接断开时收敛并通知。
	go func() {
		waitErr := s.wait()
		m.remove(id, s)
		s.shutdown()
		if m.onExit != nil {
			msg := ""
			// 用户主动关闭时客户端被强制断开，wait 的错误是预期内的，不上报。
			if !s.closedByUser.Load() && waitErr != nil && !errors.Is(waitErr, io.EOF) {
				msg = waitErr.Error()
			}
			m.onExit(id, msg)
		}
	}()

	return nil
}

// Write 向会话 stdin 写入输入（用户按键）。
func (m *Manager) Write(id string, data []byte) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	select {
	case <-s.done:
		return ErrNotFound
	default:
	}
	_, err := s.stdin.Write(data)
	return err
}

// Resize 调整 PTY 窗口尺寸。
func (m *Manager) Resize(id string, cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return errors.New("terminal size must be positive")
	}
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	return s.resize(cols, rows)
}

// Close 主动关闭会话；幂等。
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	s.closedByUser.Store(true)
	s.shutdown()
	return nil
}

// CloseAll 关闭全部会话（进程退出时调用），之后拒绝新会话。
func (m *Manager) CloseAll() {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = nil
	m.mu.Unlock()
	for _, s := range sessions {
		s.closedByUser.Store(true)
		s.shutdown()
	}
}

// remove 从表中摘除会话（由生命周期泵调用）。
// remove 只删除自己那条会话：若同 id 已被新会话占用，不能误删。
func (m *Manager) remove(id string, s *session) {
	m.mu.Lock()
	if m.sessions[id] == s {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
}

// Count 返回当前活跃会话数（诊断/测试用）。
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// splitUTF8 把数据切成「完整 UTF-8 前缀」与「未完成的尾部」。
// 不完整的尾部至多 3 字节（UTF-8 最长 4 字节序列），因此只检查末尾。
func splitUTF8(data []byte) (complete, rest []byte) {
	if utf8.Valid(data) {
		return data, nil
	}
	for tail := 1; tail <= 3 && tail < len(data); tail++ {
		if utf8.Valid(data[:len(data)-tail]) {
			return data[:len(data)-tail], data[len(data)-tail:]
		}
	}
	// 输入本身不是 UTF-8（罕见，如非 UTF-8 locale）：原样放行，前端兜底。
	return data, nil
}

// sshTerminalModes 返回常用 PTY 模式（回显 + 默认波特率）。
func sshTerminalModes() gossh.TerminalModes {
	return gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}
}
