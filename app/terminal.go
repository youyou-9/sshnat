package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/supervisor"
	"github.com/sshnat/sshnat/core/terminal"
)

// OpenTerminalRequest 描述一次远程终端会话请求。
// SessionID 可由前端预先生成，这样输出事件不会在 Open 应答前被丢弃。
type OpenTerminalRequest struct {
	HostID    string `json:"hostId"`
	Cols      int    `json:"cols,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// TerminalSession 是打开成功的会话句柄信息。
type TerminalSession struct {
	SessionID string `json:"sessionId"`
	HostID    string `json:"hostId"`
}

// Open 为指定主机开启一个交互式远程终端（PTY + shell）。
// 走与隧道完全一致的跳板链/认证/hostkey 路径。
func (t *TerminalService) Open(req OpenTerminalRequest) (*TerminalSession, error) {
	req.HostID = strings.TrimSpace(req.HostID)
	if req.HostID == "" {
		return nil, errors.New("host id is required")
	}

	t.s.tmu.Lock()
	if t.s.closing {
		t.s.tmu.Unlock()
		return nil, errors.New("application is shutting down")
	}
	settings, err := t.s.Store.Load()
	if err != nil {
		t.s.tmu.Unlock()
		return nil, err
	}
	host, ok := settings.FindHost(req.HostID)
	if !ok {
		t.s.tmu.Unlock()
		return nil, fmt.Errorf("host %s not found", req.HostID)
	}
	hostCopy := *host
	t.s.tmu.Unlock()

	opts, err := supervisor.BuildDialOptions(t.s.Store, hostCopy)
	if err != nil {
		return nil, err
	}
	timeout := 15 * time.Second
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = config.NewID("term")
	} else if len(sessionID) > 80 {
		return nil, errors.New("terminal session id is too long")
	}
	if err := t.s.Term.Open(ctx, sessionID, opts, req.Cols, req.Rows); err != nil {
		return nil, err
	}
	return &TerminalSession{SessionID: sessionID, HostID: req.HostID}, nil
}

// Write 向会话写入用户输入。
func (t *TerminalService) Write(sessionID, data string) error {
	return t.s.Term.Write(strings.TrimSpace(sessionID), []byte(data))
}

// Resize 调整 PTY 尺寸。
func (t *TerminalService) Resize(sessionID string, cols, rows int) error {
	return t.s.Term.Resize(strings.TrimSpace(sessionID), cols, rows)
}

// Close 关闭会话（幂等）。
func (t *TerminalService) Close(sessionID string) error {
	err := t.s.Term.Close(strings.TrimSpace(sessionID))
	if errors.Is(err, terminal.ErrNotFound) {
		return nil // 已关闭视为成功，前端无需感知
	}
	return err
}
