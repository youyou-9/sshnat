// Package version contains build-time application metadata.
//
// Release builds inject Version with Go's -ldflags -X option. Keeping a
// useful default makes local development binaries self-describing while
// allowing all UI and service surfaces to report the same value.
package version

import "strings"

// Name is the product name shown by the desktop client and daemon tooling.
const Name = "SSHNat"

// Version is replaced at build time with the release version (without the
// leading "v"). Local builds intentionally report "dev".
var Version = "dev"

// Current returns a display-safe version string. Accepting a leading "v"
// keeps manual -ldflags invocations and CI tags equivalent.
func Current() string {
	v := strings.TrimPrefix(strings.TrimSpace(Version), "v")
	if v == "" {
		return "dev"
	}
	return v
}
