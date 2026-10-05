package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// Store 管理配置文件的读写。
type Store struct {
	path string
}

// NewStore 创建绑定到指定路径的 Store。
func NewStore(path string) *Store { return &Store{path: path} }

// DefaultPath 返回默认配置路径。
// 优先将 config.json 放在可执行文件所在目录（便于单目录便携运行与管理）；
// 若可执行文件目录不可写，则回退到系统用户配置目录（如 AppData）。
func DefaultPath() (string, error) {
	if custom := os.Getenv("SSHNAT_CONFIG_DIR"); custom != "" {
		return filepath.Join(custom, "config.json"), nil
	}
	if custom := os.Getenv("SSHNA_CONFIG_DIR"); custom != "" {
		return filepath.Join(custom, "config.json"), nil
	}

	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		target := filepath.Join(exeDir, "config.json")

		// 在 macOS 上，若处于 .app Bundle 内部 (.app/Contents/MacOS)，不可向 App 内部写入数据（避免破坏签名与沙盒权限）
		isMacAppBundle := strings.Contains(exeDir, ".app/Contents/MacOS")

		if !isMacAppBundle && isWritableDir(exeDir) {
			// 若可执行文件同目录下尚未存在 config.json，但旧 AppData 目录中存在已配置的文件，自动平滑复制迁移
			if _, statErr := os.Stat(target); errors.Is(statErr, os.ErrNotExist) {
				if userDir, uerr := os.UserConfigDir(); uerr == nil {
					oldConfig := filepath.Join(userDir, "sshnat", "config.json")
					if oldData, rerr := os.ReadFile(oldConfig); rerr == nil && len(oldData) > 0 {
						_ = os.WriteFile(target, oldData, 0o600)
						oldKnown := filepath.Join(userDir, "sshnat", "known_hosts")
						if kData, kerr := os.ReadFile(oldKnown); kerr == nil {
							_ = os.WriteFile(filepath.Join(exeDir, "known_hosts"), kData, 0o600)
						}
					}
				}
			}
			return target, nil
		}
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve user config dir: %w", err)
	}
	return filepath.Join(base, "sshnat", "config.json"), nil
}

func isWritableDir(dir string) bool {
	testFile := filepath.Join(dir, fmt.Sprintf(".sshnat_write_test_%d.tmp", nowUnixNano()))
	if err := os.WriteFile(testFile, []byte(""), 0o600); err != nil {
		return false
	}
	_ = os.Remove(testFile)
	return true
}

// Path 返回配置文件路径。
func (s *Store) Path() string { return s.path }

// Load 读取并解析配置；文件不存在时返回零值设置与 nil 错误。
func (s *Store) Load() (*Settings, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Settings{Version: 1}, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", s.path, err)
	}
	st, err := decodeSettings(data)
	if err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", s.path, err)
	}
	return st, nil
}

// Save 原子写入配置（先写独立临时文件再 rename），权限 0600。
func (s *Store) Save(st *Settings) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("config: chmod temp: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("config: write temp: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("config: sync temp: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("config: close temp: %w", err)
	}
	if err := replaceFile(tmpPath, s.path); err != nil {
		return fmt.Errorf("config: replace: %w", err)
	}
	return nil
}

// NewID 生成一个简单唯一 ID。为避免引入额外依赖，使用时间戳+计数器；
// 单进程内保证唯一，跨设备同步场景未来可换 UUID。
var idCounter atomic.Int64

// NewID 返回形如 "t-1718000000000000000" 的 ID。
func NewID(prefix string) string {
	n := idCounter.Add(1)
	return fmt.Sprintf("%s-%d%04d", prefix, nowUnixNano(), n%10000)
}
