//go:build windows

package startup

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procMessageBoxW = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

const (
	mbIconWarning   = 0x00000030
	mbSetForeground = 0x00010000
	mbTopMost       = 0x00040000
)

// Alert shows a warning message box. It is the last thing the process does
// before exiting, so blocking until the user dismisses it is intentional: an
// explanation nobody reads is better than a window that silently disappears.
func Alert(title, body string) {
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	b, err := windows.UTF16PtrFromString(body)
	if err != nil {
		return
	}
	_, _, _ = procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(b)),
		uintptr(unsafe.Pointer(t)),
		mbIconWarning|mbSetForeground|mbTopMost,
	)
}
