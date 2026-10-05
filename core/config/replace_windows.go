//go:build windows

package config

import "golang.org/x/sys/windows"

// replaceFile uses MoveFileEx with REPLACE_EXISTING because os.Rename on
// Windows rejects an existing destination. WRITE_THROUGH makes the replacement
// durable before Save returns, matching Store's fsync of the temporary file.
func replaceFile(src, dst string) error {
	srcPath, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstPath, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(srcPath, dstPath, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
