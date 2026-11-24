//go:build !windows

package startup

import "log"

// Alert writes the message to the log on platforms that never need it: the
// integrity-level guard only fires where mandatory labels exist.
func Alert(title, body string) {
	log.Printf("startup aborted: %s: %s", title, body)
}
