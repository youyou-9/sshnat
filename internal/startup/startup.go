// Package startup owns the first moments of the desktop client: the
// diagnostic log file and the pre-flight checks that have to run before the
// WebView window exists.
//
// The checks exist because a WebView2 window can fail before the UI has any
// chance to report anything. On Windows the usual cause is the mandatory
// integrity level: WebView2 refuses to attach its medium-integrity browser
// process to a host running below medium, and Windows launches an executable
// at low integrity whenever the folder that contains it carries a low
// mandatory label (some sandboxing tools tag their workspace that way). The
// symptom is brutal — the process exits after a few hundred milliseconds with
// no window, no tray icon and no message — so SSHNat detects the condition up
// front and explains it instead of dying silently.
package startup

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// Level is the Windows mandatory integrity level of the current process.
// Platforms without integrity levels always report LevelMedium.
type Level int

const (
	LevelUntrusted Level = iota
	LevelLow
	LevelMedium
	LevelHigh
	LevelSystem
)

// webViewMinimumLevel is the lowest integrity level at which WebView2 will
// attach a controller to a host window.
const webViewMinimumLevel = LevelMedium

// String renders the level the way Windows names it in icacls output.
func (l Level) String() string {
	switch l {
	case LevelUntrusted:
		return "untrusted"
	case LevelLow:
		return "low"
	case LevelMedium:
		return "medium"
	case LevelHigh:
		return "high"
	case LevelSystem:
		return "system"
	default:
		return fmt.Sprintf("unknown(%d)", int(l))
	}
}

// AllowsWebView2 reports whether a process at this integrity level can host a
// WebView2 window.
func (l Level) AllowsWebView2() bool { return l >= webViewMinimumLevel }

// LevelFromRID maps the relative identifier of a mandatory label SID
// (S-1-16-<rid>) to its level. Values above the documented bands are treated
// as system, which keeps protected-process labels on the safe side.
func LevelFromRID(rid uint32) Level {
	switch {
	case rid >= 0x4000:
		return LevelSystem
	case rid >= 0x3000:
		return LevelHigh
	case rid >= 0x2000:
		return LevelMedium
	case rid >= 0x1000:
		return LevelLow
	default:
		return LevelUntrusted
	}
}

// maxLogBytes caps the startup log so repeated launches cannot grow it without
// bound. The file is rotated once per launch at most.
const maxLogBytes = 1 << 20

// LogPath returns the file that receives startup diagnostics. It lives next to
// the WebView2 profile under the user's cache directory, which is the one
// location the desktop client is guaranteed to be able to write to.
func LogPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "SSHNat", "logs", "startup.log"), nil
}

// Begin redirects the standard logger to the startup log and returns the
// function that closes it together with the path that was actually opened.
//
// The default location is not always writable: a low-integrity process — which
// is exactly the case this package cares about — cannot write into
// %LOCALAPPDATA%. Begin therefore falls back to the temp directory and, when
// neither works, reports the error and an empty path so the caller can say so
// in the abort dialog instead of pointing at a file that does not exist.
func Begin(banner string) (stop func(), path string, err error) {
	primary, pathErr := LogPath()
	if pathErr == nil {
		stop, openErr := Open(primary, banner)
		if openErr == nil {
			return stop, primary, nil
		}
		pathErr = openErr
	}

	fallback := filepath.Join(os.TempDir(), "SSHNat", "startup.log")
	if stop, openErr := Open(fallback, banner); openErr == nil {
		return stop, fallback, nil
	}
	return func() {}, "", pathErr
}

// Open redirects the standard logger to path and returns the function that
// closes it. Wails reports WebView2 failures through the standard logger, so
// capturing it here is what makes a silent start-up failure diagnosable after
// the fact.
func Open(path, banner string) (stop func(), err error) {
	noop := func() {}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return noop, err
	}
	if info, statErr := os.Stat(path); statErr == nil && info.Size() > maxLogBytes {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return noop, err
	}
	// The file comes first: a GUI build has no usable stderr handle, and
	// io.MultiWriter stops at the first writer that errors.
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("=== %s", banner)
	log.Printf("log file: %s", path)
	return func() {
		log.Printf("=== exiting")
		_ = f.Close()
	}, nil
}
