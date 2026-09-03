package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestKeyboardInteractiveOnlyAnswersPassword(t *testing.T) {
	cb := keyboardInteractive("user", "my-secret-password")

	// 密码提问正常返回密码
	ans, err := cb("name", "inst", []string{"Password:", "Enter password for user:"}, []bool{false, false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ans) != 2 || ans[0] != "my-secret-password" || ans[1] != "my-secret-password" {
		t.Fatalf("unexpected answers: %v", ans)
	}

	// 多重验证提示（如 OTP / Verification Code）不应当将密码泄露进去
	ans2, err := cb("name", "inst", []string{"Verification Code (OTP):", "Password:"}, []bool{false, false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans2[0] != "" {
		t.Fatalf("OTP prompt should not get password: got %q", ans2[0])
	}
	if ans2[1] != "my-secret-password" {
		t.Fatalf("Password prompt should get password: got %q", ans2[1])
	}
}

func TestHostKeyCallbackAcceptNew(t *testing.T) {
	dir := t.TempDir()
	khFile := filepath.Join(dir, "known_hosts")

	opts := &DialOptions{
		KnownHostsFile: khFile,
	}
	cb, err := hostKeyCallback(opts)
	if err != nil {
		t.Fatalf("hostKeyCallback: %v", err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pub := signer.PublicKey()

	dummyAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}

	// 首次连接：accept-new 写入 known_hosts
	if err := cb("127.0.0.1:22", dummyAddr, pub); err != nil {
		t.Fatalf("accept-new failed: %v", err)
	}

	content, err := os.ReadFile(khFile)
	if err != nil || len(content) == 0 {
		t.Fatalf("known_hosts should have entries: %v", err)
	}

	// 再次校验同个 key 应当直接成功
	cb2, err := hostKeyCallback(opts)
	if err != nil {
		t.Fatalf("hostKeyCallback 2: %v", err)
	}
	if err := cb2("127.0.0.1:22", dummyAddr, pub); err != nil {
		t.Fatalf("second check failed: %v", err)
	}
}
