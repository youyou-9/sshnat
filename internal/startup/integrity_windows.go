//go:build windows

package startup

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IntegrityLevel returns the mandatory integrity level of the running process.
func IntegrityLevel() (Level, error) {
	token := windows.GetCurrentProcessToken()

	// The label is a SID plus attributes; 128 bytes covers the fixed struct and
	// any label SID Windows ships today.
	buf := make([]byte, 128)
	var size uint32
	if err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buf[0], uint32(len(buf)), &size); err != nil {
		return LevelUntrusted, fmt.Errorf("read token integrity level: %w", err)
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buf[0]))
	sid := label.Label.Sid
	if sid == nil || sid.SubAuthorityCount() == 0 {
		return LevelUntrusted, fmt.Errorf("integrity label SID is empty")
	}
	return LevelFromRID(sid.SubAuthority(uint32(sid.SubAuthorityCount() - 1))), nil
}
