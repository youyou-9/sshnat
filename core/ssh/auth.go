package ssh

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/sshnat/sshnat/core/internal/agentdial"
)

// newClientConfig 组装握手所需的 ClientConfig（hostkey + 认证）。
func newClientConfig(opts *DialOptions) (*sshClientConfig, error) {
	hkc, err := hostKeyCallback(opts)
	if err != nil {
		return nil, err
	}

	methods, err := buildAuthMethods(&opts.Auth, opts.User)
	if err != nil {
		return nil, err
	}

	return &sshClientConfig{
		User:            opts.User,
		Auth:            methods,
		HostKeyCallback: hkc,
		Timeout:         opts.timeout(),
	}, nil
}

// buildAuthMethods 按配置生成认证方法列表。
// 密码方式自动附带 keyboard-interactive 回退；私钥支持加密私钥。
func buildAuthMethods(auth *AuthConfig, user string) ([]gossh.AuthMethod, error) {
	var methods []gossh.AuthMethod

	switch auth.Type {
	case AuthPassword:
		pwd := auth.Password
		methods = append(methods,
			gossh.Password(pwd),
			gossh.KeyboardInteractive(keyboardInteractive(user, pwd)),
		)
	case AuthKey:
		if auth.KeyPath == "" {
			return nil, errors.New("ssh: key path is empty")
		}
		signers, err := loadPrivateKeySigners(auth.KeyPath, auth.KeyPassphrase)
		if err != nil {
			return nil, err
		}
		methods = append(methods, gossh.PublicKeys(signers...))
	case AuthAgent:
		conn, err := agentdial.Dial(auth.AgentSocket)
		if err != nil {
			return nil, fmt.Errorf("ssh: connect agent: %w", err)
		}
		agentClient := agent.NewClient(conn)
		methods = append(methods, gossh.PublicKeysCallback(func() ([]gossh.Signer, error) {
			return agentClient.Signers()
		}))
	default:
		return nil, fmt.Errorf("ssh: unknown auth type %q", auth.Type)
	}

	if len(methods) == 0 {
		return nil, errors.New("ssh: no auth method configured")
	}
	return methods, nil
}

// keyboardInteractive 对密码类提问以密码作答，忽略未知提问（避免将密码误填入二次验证码）。
func keyboardInteractive(user, password string) gossh.KeyboardInteractiveChallenge {
	return func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i, q := range questions {
			if containsFold(q, "password") || containsFold(q, "passphrase") || containsFold(q, "密码") || len(questions) == 1 {
				answers[i] = password
			}
		}
		return answers, nil
	}
}

func containsFold(s, sub string) bool {
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		if foldEqual(s[i:i+n], sub) {
			return true
		}
	}
	return false
}

func foldEqual(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// loadPrivateKeySigners 读取私钥文件（支持加密私钥）并返回 signer 列表。
func loadPrivateKeySigners(path, passphrase string) ([]gossh.Signer, error) {
	realPath := expandPath(path)
	data, err := os.ReadFile(realPath)
	if err != nil {
		return nil, fmt.Errorf("ssh: read key %s: %w", realPath, err)
	}
	var key gossh.Signer
	if passphrase == "" {
		key, err = gossh.ParsePrivateKey(data)
	} else {
		key, err = gossh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	}
	if err != nil {
		if isPassphraseMissing(err) {
			return nil, fmt.Errorf("ssh: private key %s requires a passphrase", path)
		}
		return nil, fmt.Errorf("ssh: parse key %s: %w", path, err)
	}
	return []gossh.Signer{key}, nil
}

type passphraseMissingError struct{}

func (passphraseMissingError) Error() string { return "ssh: this private key is passphrase protected" }

func isPassphraseMissing(err error) bool {
	var pme *gossh.PassphraseMissingError
	return errors.As(err, &pme)
}
