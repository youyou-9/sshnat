package ssh

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyCallback 返回 hostkey 校验回调：
//   - InsecureSkipHostKey：跳过（仅测试）
//   - KnownHostsFile 已有条目 → 严格校验
//   - 未知主机 → accept-new：记录指纹并接受，写入 known_hosts
func hostKeyCallback(opts *DialOptions) (gossh.HostKeyCallback, error) {
	if opts.InsecureSkipHostKey {
		return gossh.InsecureIgnoreHostKey(), nil //nolint:gosec // 显式开关，仅测试环境
	}

	file := opts.KnownHostsFile
	if file == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("ssh: resolve home directory for known_hosts: %w", err)
		}
		file = filepath.Join(home, ".ssh", "known_hosts")
	}

	if _, err := os.Stat(file); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("ssh: stat known_hosts: %w", err)
		}
		if mkErr := os.MkdirAll(filepath.Dir(file), 0o700); mkErr != nil {
			return nil, fmt.Errorf("ssh: create known_hosts dir: %w", mkErr)
		}
		// 文件不存在时安全创建空文件，避免并发 O_TRUNC 导致已写入公钥丢失。
		f, crErr := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if crErr == nil {
			_ = f.Close()
		} else if !errors.Is(crErr, os.ErrExist) {
			return nil, fmt.Errorf("ssh: create known_hosts: %w", crErr)
		}
	}

	strict, strictErr := knownhosts.New(file)
	if strictErr != nil {
		return nil, fmt.Errorf("ssh: load known_hosts: %w", strictErr)
	}

	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		err := strict(hostname, remote, key)
		if err == nil {
			return nil
		}
		var kerr *knownhosts.KeyError
		if errors.As(err, &kerr) && len(kerr.Want) == 0 {
			// 未知主机 → accept-new
			line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
			f, ferr := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if ferr != nil {
				return fmt.Errorf("ssh: record new host: %w", ferr)
			}
			defer f.Close()
			if _, werr := f.WriteString(line + "\n"); werr != nil {
				return fmt.Errorf("ssh: record new host: %w", werr)
			}
			return nil
		}
		// 主机密钥变更等严格失败。
		return fmt.Errorf("ssh: %w", err)
	}, nil
}
