package supervisor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/ssh"
)

// buildDialOptions 把 config.Host（含跳板链）转换为 core/ssh 的拨号参数。
// 跳板链按 ID 递归展开，带环检测。
func buildDialOptions(store *config.Store, host config.Host) (ssh.DialOptions, error) {
	settings, err := store.Load()
	if err != nil {
		return ssh.DialOptions{}, err
	}
	knownHosts := filepath.Join(filepath.Dir(store.Path()), "known_hosts")
	return buildDialOptionsRec(settings, host, map[string]bool{host.ID: true}, knownHosts)
}

func buildDialOptionsRec(settings *config.Settings, host config.Host, visiting map[string]bool, defaultKnownHosts string) (ssh.DialOptions, error) {
	opts := ssh.DialOptions{
		Host:           host.Host,
		Port:           host.Port,
		User:           host.User,
		KnownHostsFile: host.KnownHostsFile,
		HostKeyPolicy:  host.HostKeyPolicy,
		Timeout:        time.Duration(host.ConnectTimeoutSeconds) * time.Second,
	}
	if opts.KnownHostsFile == "" {
		opts.KnownHostsFile = defaultKnownHosts
	}
	// A zero value means "use the documented default", rather than silently
	// disabling health checks. A negative value can explicitly disable it in a
	// hand-written configuration.
	if host.KeepaliveSeconds == 0 {
		opts.Keepalive = 15 * time.Second
	} else if host.KeepaliveSeconds > 0 {
		opts.Keepalive = time.Duration(host.KeepaliveSeconds) * time.Second
	} else {
		opts.Keepalive = -1
	}

	switch host.Auth.Method {
	case "", config.AuthMethodPassword:
		opts.Auth = ssh.AuthConfig{
			Type:     ssh.AuthPassword,
			Password: host.Auth.Password,
		}
	case config.AuthMethodKey:
		opts.Auth = ssh.AuthConfig{
			Type:          ssh.AuthKey,
			KeyPath:       host.Auth.KeyPath,
			KeyPassphrase: host.Auth.KeyPassphrase,
		}
	case config.AuthMethodAgent:
		opts.Auth = ssh.AuthConfig{
			Type:        ssh.AuthAgent,
			AgentSocket: host.Auth.AgentSocket,
		}
	default:
		return ssh.DialOptions{}, fmt.Errorf("supervisor: unknown auth method %q", host.Auth.Method)
	}

	if n := len(host.JumpHostIDs); n > 0 {
		opts.JumpHosts = make([]ssh.DialOptions, 0, n)
		for _, jid := range host.JumpHostIDs {
			if jid == "" {
				continue
			}
			if visiting[jid] {
				return ssh.DialOptions{}, errors.New("supervisor: jump chain contains a cycle")
			}
			jump, ok := settings.FindHost(jid)
			if !ok {
				return ssh.DialOptions{}, fmt.Errorf("supervisor: jump host %s not found", jid)
			}
			visiting[jid] = true
			jopts, err := buildDialOptionsRec(settings, *jump, visiting, defaultKnownHosts)
			delete(visiting, jid)
			if err != nil {
				return ssh.DialOptions{}, fmt.Errorf("supervisor: resolve jump %s: %w", jump.Name, err)
			}
			// A jump host may itself use ProxyJump. Flatten that chain so the
			// SSH layer receives one nearest-to-farthest sequence.
			opts.JumpHosts = appendFlattened(opts.JumpHosts, jopts)
		}
	}
	return opts, nil
}

func appendFlattened(dst []ssh.DialOptions, opts ssh.DialOptions) []ssh.DialOptions {
	for _, nested := range opts.JumpHosts {
		dst = append(dst, nested)
	}
	opts.JumpHosts = nil
	return append(dst, opts)
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
