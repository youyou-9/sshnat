package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sshnat/sshnat/internal/autostart"
	"github.com/sshnat/sshnat/internal/update"
	"github.com/sshnat/sshnat/internal/version"
)

// PickFile 打开原生文件选择器（选择私钥/known_hosts 等），取消返回空字符串。
func (st *SettingsService) PickFile() (string, error) {
	st.s.emu.RLock()
	choose := st.s.chooseFile
	st.s.emu.RUnlock()
	if choose == nil {
		return "", errors.New("native open dialog is unavailable")
	}
	return choose()
}

// GetLanguage 返回持久化的界面语言（"zh" | "en"）；未设置时为空字符串。
func (st *SettingsService) GetLanguage() (string, error) {
	settings, err := st.s.Store.Load()
	if err != nil {
		return "", err
	}
	return settings.Language, nil
}

// SetLanguage 持久化界面语言，托盘等原生 UI 随之切换。
func (st *SettingsService) SetLanguage(language string) error {
	language = strings.TrimSpace(language)
	if language != "zh" && language != "en" {
		return fmt.Errorf("unsupported language %q", language)
	}
	st.s.tmu.Lock()
	changed := false
	defer func() {
		st.s.tmu.Unlock()
		if changed {
			st.s.emitConfigChanged()
		}
	}()
	settings, err := st.s.Store.Load()
	if err != nil {
		return err
	}
	if settings.Language == language {
		return nil
	}
	settings.Language = language
	if err := st.s.Store.Save(settings); err != nil {
		return err
	}
	changed = true
	return nil
}

// GetAutoStart 报告是否已注册开机自启动。
func (st *SettingsService) GetAutoStart() (bool, error) {
	return autostart.IsEnabled(), nil
}

// SetAutoStart 开启或关闭开机自启动。
func (st *SettingsService) SetAutoStart(enabled bool) error {
	if enabled {
		return autostart.Enable()
	}
	return autostart.Disable()
}

// UpdateInfo 是一次更新检查的结果（字段与 internal/update.Info 对齐）。
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
	HasUpdate bool   `json:"hasUpdate"`
	// Reason 是结果分类（up-to-date/available/no-release/dev-build/unparsable），
	// 前端据此本地化展示，不依赖英文文案。
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// CheckUpdate 查询 GitHub Releases 上的最新版本并与当前版本比较。
func (st *SettingsService) CheckUpdate() (*UpdateInfo, error) {
	info, err := update.Check(context.Background(), version.Current())
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("update check returned no result")
	}
	return &UpdateInfo{
		Current:   info.Current,
		Latest:    info.Latest,
		URL:       info.URL,
		HasUpdate: info.HasUpdate,
		Reason:    string(info.Reason),
		Message:   info.Message,
	}, nil
}
