package startup

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLevelFromRIDMapsWindowsBands(t *testing.T) {
	cases := []struct {
		name string
		rid  uint32
		want Level
	}{
		{name: "untrusted", rid: 0x0000, want: LevelUntrusted},
		{name: "low", rid: 0x1000, want: LevelLow},
		{name: "medium", rid: 0x2000, want: LevelMedium},
		{name: "high", rid: 0x3000, want: LevelHigh},
		{name: "system", rid: 0x4000, want: LevelSystem},
		{name: "protected process", rid: 0x5000, want: LevelSystem},
		{name: "inside the low band", rid: 0x1100, want: LevelLow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LevelFromRID(tc.rid); got != tc.want {
				t.Fatalf("LevelFromRID(%#x) = %v, want %v", tc.rid, got, tc.want)
			}
		})
	}
}

func TestAllowsWebView2OnlyFromMediumUp(t *testing.T) {
	allowed := map[Level]bool{
		LevelUntrusted: false,
		LevelLow:       false,
		LevelMedium:    true,
		LevelHigh:      true,
		LevelSystem:    true,
	}
	for level, want := range allowed {
		if got := level.AllowsWebView2(); got != want {
			t.Fatalf("%v.AllowsWebView2() = %v, want %v", level, got, want)
		}
	}
}

func TestAbortMessageNamesFolderLogAndFix(t *testing.T) {
	report := Report{
		Exe:     `D:\workspace\SSHNat.exe`,
		ExeDir:  `D:\workspace`,
		Level:   LevelLow,
		LogPath: `C:\Users\me\AppData\Local\SSHNat\logs\startup.log`,
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			title, body := AbortMessage(lang, report)
			if title == "" {
				t.Fatal("title is empty")
			}
			for _, want := range []string{report.ExeDir, report.LogPath, "icacls", "/setintegritylevel", "low"} {
				if !strings.Contains(body, want) {
					t.Fatalf("body is missing %q:\n%s", want, body)
				}
			}
			if !strings.Contains(body, "%LOCALAPPDATA%") {
				t.Fatalf("body should show the recommended install folder:\n%s", body)
			}
		})
	}
}

func TestAbortMessageSurvivesMissingPaths(t *testing.T) {
	for _, lang := range []string{"zh", "en", ""} {
		title, body := AbortMessage(lang, Report{Level: LevelUntrusted})
		if title == "" || body == "" {
			t.Fatalf("lang %q produced an empty message", lang)
		}
		if !strings.Contains(body, "unknown") && !strings.Contains(body, "不可用") && !strings.Contains(body, "unavailable") {
			t.Fatalf("lang %q should mention the missing detail:\n%s", lang, body)
		}
	}
}

func TestLogPathLivesUnderUserCacheDir(t *testing.T) {
	path, err := LogPath()
	if err != nil {
		t.Fatalf("LogPath: %v", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache dir: %v", err)
	}
	want := filepath.Join(cache, "SSHNat", "logs", "startup.log")
	if path != want {
		t.Fatalf("LogPath() = %q, want %q", path, want)
	}
}

func TestAbortMessageExplainsMissingLog(t *testing.T) {
	cases := map[string]string{"zh": "不可用", "en": "unavailable"}
	for lang, want := range cases {
		t.Run(lang, func(t *testing.T) {
			_, body := AbortMessage(lang, Report{ExeDir: `D:\blocked`, Level: LevelLow})
			if !strings.Contains(body, want) {
				t.Fatalf("body should explain the missing log with %q:\n%s", want, body)
			}
			if strings.Contains(body, "startup.log") {
				t.Fatalf("body must not name a log file that was never written:\n%s", body)
			}
		})
	}
}

func TestBeginFallsBackWhenDefaultIsUnwritable(t *testing.T) {
	if runtime.GOOS == "darwin" {
		// os.UserCacheDir ignores XDG_CACHE_HOME on macOS, so the primary path
		// cannot be redirected into the test sandbox there.
		t.Skip("os.UserCacheDir is not redirectable on macOS")
	}
	base := t.TempDir()
	local := filepath.Join(base, "local")
	tmp := filepath.Join(base, "tmp")
	for key, value := range map[string]string{
		"LOCALAPPDATA":   local, // Windows cache dir
		"XDG_CACHE_HOME": local, // Linux cache dir
		"TEMP":           tmp,
		"TMP":            tmp,
		"TMPDIR":         tmp,
	} {
		t.Setenv(key, value)
	}

	// A directory where the default log file should be makes the primary open
	// fail the same way a low-integrity process does.
	blocked := filepath.Join(local, "SSHNat", "logs", "startup.log")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatalf("prepare blocked path: %v", err)
	}

	original := log.Writer()
	t.Cleanup(func() { log.SetOutput(original) })

	stop, path, err := Begin("fallback banner")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	stop()

	want := filepath.Join(tmp, "SSHNat", "startup.log")
	if path != want {
		t.Fatalf("Begin path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read fallback log: %v", err)
	}
	if !strings.Contains(string(data), "fallback banner") {
		t.Fatalf("fallback log is missing the banner:\n%s", data)
	}
}

func TestOpenWritesBannerAndCloses(t *testing.T) {
	original := log.Writer()
	originalFlags := log.Flags()
	t.Cleanup(func() {
		log.SetOutput(original)
		log.SetFlags(originalFlags)
	})

	path := filepath.Join(t.TempDir(), "logs", "startup.log")
	stop, err := Open(path, "SSHNat test starting")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	log.Printf("[WebView2 Error] simulated failure")
	stop()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, want := range []string{"SSHNat test starting", "simulated failure", "exiting"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("log is missing %q:\n%s", want, data)
		}
	}
}

func TestOpenRotatesOversizedLog(t *testing.T) {
	original := log.Writer()
	t.Cleanup(func() { log.SetOutput(original) })

	dir := t.TempDir()
	path := filepath.Join(dir, "startup.log")
	if err := os.WriteFile(path, make([]byte, maxLogBytes+1), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	stop, err := Open(path, "after rotation")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stop()

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated log: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fresh log: %v", err)
	}
	if !strings.Contains(string(data), "after rotation") {
		t.Fatalf("fresh log is missing the banner:\n%s", data)
	}
}
