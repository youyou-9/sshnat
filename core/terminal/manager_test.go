package terminal

import (
	"bytes"
	"testing"

	"github.com/sshnat/sshnat/core/ssh"
)

// dialOptionsStub 仅用于编译期填充；测试路径在拨号前就会返回。
func dialOptionsStub() ssh.DialOptions { return ssh.DialOptions{} }

func TestSplitUTF8(t *testing.T) {
	zh := []byte("中文输出") // 每个汉字 3 字节
	for _, test := range []struct {
		name         string
		data         []byte
		wantComplete int
		wantRest     int
	}{
		{"ascii", []byte("hello"), 5, 0},
		{"full multibyte", zh, len(zh), 0},
		{"cut tail", zh[:len(zh)-1], len(zh) - 3, 2},
		{"cut tail two bytes", zh[:len(zh)-2], len(zh) - 3, 1},
		{"mixed", append([]byte("ls\n"), zh...), 3 + len(zh), 0},
	} {
		complete, rest := splitUTF8(test.data)
		if len(complete) != test.wantComplete || len(rest) != test.wantRest {
			t.Fatalf("%s: split = (%d, %d), want (%d, %d)", test.name, len(complete), len(rest), test.wantComplete, test.wantRest)
		}
		if !bytes.Equal(append(complete, rest...), test.data) {
			t.Fatalf("%s: split lost bytes", test.name)
		}
	}
}

func TestCloseUnknownSessionIsNotFound(t *testing.T) {
	m := NewManager(nil, nil)
	if err := m.Close("nope"); err != ErrNotFound {
		t.Fatalf("Close = %v, want ErrNotFound", err)
	}
	if err := m.Write("nope", []byte("x")); err != ErrNotFound {
		t.Fatalf("Write = %v, want ErrNotFound", err)
	}
	if err := m.Resize("nope", 80, 24); err != ErrNotFound {
		t.Fatalf("Resize = %v, want ErrNotFound", err)
	}
}

func TestCloseAllRejectsNewSessions(t *testing.T) {
	m := NewManager(nil, nil)
	m.CloseAll()
	if err := m.Open(t.Context(), "s1", dialOptionsStub(), 80, 24); err != ErrClosing {
		t.Fatalf("Open after CloseAll = %v, want ErrClosing", err)
	}
}
