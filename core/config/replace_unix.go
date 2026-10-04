//go:build !windows

package config

import "os"

// replaceFile atomically replaces dst on platforms where rename replaces an
// existing destination (Unix and Unix-like systems).
func replaceFile(src, dst string) error { return os.Rename(src, dst) }
