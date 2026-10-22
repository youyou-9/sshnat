package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The WebView2 profile directory has to be a stable, predictable path: the
// frontend keeps its UI preferences there, and a corrupted store inside it is
// fatal for the boot sequence unless we can point at it.
func TestWebviewDataPathIsPredictable(t *testing.T) {
	path := webviewDataPath()
	if path == "" {
		// os.UserConfigDir() can legitimately fail (no HOME/APPDATA); the app
		// then falls back to the Wails default, so there is nothing to assert.
		t.Skip("user config directory is unavailable in this environment")
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("webviewDataPath() = %q, want an absolute path", path)
	}
	clean := filepath.ToSlash(path)
	if !strings.HasSuffix(clean, "SSHNat/webview") {
		t.Fatalf("webviewDataPath() = %q, want a path ending in SSHNat/webview", path)
	}
}
