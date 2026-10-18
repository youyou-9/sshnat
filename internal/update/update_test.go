package update

import "testing"

func TestCompareVersions(t *testing.T) {
	for _, test := range []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.3.0", "1.2.3", 1},
		{"1.2.3", "v1.3.0", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.2", "1.2.0", 0},
		{"1.0.0-rc.1", "1.0.0", -1},     // 正式版大于预发布版
		{"1.0.0", "1.0.0-rc.1", 1},      // 反过来同理
		{"1.0.0-rc.2", "1.0.0-rc.1", 1}, // 预发布数字段按数值比较
		{"1.0.0-rc.10", "1.0.0-rc.9", 1},
		{"1.0.0-beta", "1.0.0-rc", -1}, // 字母段按字典序
		{"0.9.9", "1.0.0", -1},
	} {
		got, err := compareVersions(test.a, test.b)
		if err != nil {
			t.Fatalf("compareVersions(%q, %q): %v", test.a, test.b, err)
		}
		if got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}

func TestCompareVersionsRejectsGarbage(t *testing.T) {
	if _, err := compareVersions("abc", "1.0.0"); err == nil {
		t.Fatal("expected error for non-numeric version")
	}
	if _, err := compareVersions("1.0.0.0", "1.0.0"); err == nil {
		t.Fatal("expected error for four-segment version")
	}
}

func TestNormalize(t *testing.T) {
	if got := normalize("  v1.2.3 "); got != "1.2.3" {
		t.Fatalf("normalize = %q", got)
	}
}
