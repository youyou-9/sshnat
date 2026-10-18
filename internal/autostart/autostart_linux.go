//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func desktopEntryPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "autostart", "sshnat.desktop"), nil
}

func platformIsEnabled(exe string) bool {
	path, err := desktopEntryPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "Exec="+exe)
}

func platformEnable(exe string) error {
	path, err := desktopEntryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create autostart directory: %w", err)
	}
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=SSHNat
Comment=SSH port forwarding client
Exec=%s
Terminal=false
X-GNOME-Autostart-enabled=true
`, exe)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write desktop entry: %w", err)
	}
	return nil
}

func platformDisable() error {
	path, err := desktopEntryPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove desktop entry: %w", err)
	}
	return nil
}
