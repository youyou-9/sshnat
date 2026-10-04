// sshnat-buildmeta keeps native package metadata aligned with a release tag.
// It is build tooling only and is never linked into sshnatd.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var releaseVersion = regexp.MustCompile(`^v?([0-9]+\.[0-9]+\.[0-9]+)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type replacement struct {
	path    string
	pattern *regexp.Regexp
	value   string
}

func main() {
	version := flag.String("version", "dev", "release version or tag, e.g. 1.2.3 or v1.2.3")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if err := syncMetadata(*root, *version); err != nil {
		fmt.Fprintf(os.Stderr, "sync release metadata: %v\n", err)
		os.Exit(1)
	}
}

func syncMetadata(root, version string) error {
	version = strings.TrimSpace(version)
	if version == "dev" {
		return nil // Development builds preserve the last release's package metadata.
	}
	match := releaseVersion.FindStringSubmatch(version)
	if match == nil {
		return fmt.Errorf("invalid release version %q (expected major.minor.patch)", version)
	}
	// Native version fields accept only numeric components. Runtime build
	// metadata continues to use the full version including a prerelease suffix.
	numeric := match[1]
	for _, component := range strings.Split(numeric, ".") {
		n, err := strconv.Atoi(component)
		if err != nil || n > 65535 {
			return fmt.Errorf("native package version component %q exceeds 65535", component)
		}
	}
	replacements := []replacement{
		{"build/config.yml", regexp.MustCompile(`(?m)^(  version: ")[^"]+(".*)$`), "${1}" + numeric + "${2}"},
		{"build/darwin/Info.plist", regexp.MustCompile(`(?s)(<key>CFBundle(?:ShortVersionString|Version)</key>\s*<string>)[^<]+(</string>)`), "${1}" + numeric + "${2}"},
		{"build/darwin/Info.dev.plist", regexp.MustCompile(`(?s)(<key>CFBundle(?:ShortVersionString|Version)</key>\s*<string>)[^<]+(</string>)`), "${1}" + numeric + "${2}"},
		{"build/windows/info.json", regexp.MustCompile(`("(?:file_version|FileVersion|ProductVersion)"\s*:\s*")[^"]+(")`), "${1}" + numeric + "${2}"},
		{"build/windows/wails.exe.manifest", regexp.MustCompile(`(<assemblyIdentity type="win32"[^>]* version=")[^"]+("[^>]*>)`), "${1}" + numeric + ".0${2}"},
		{"build/windows/msix/app_manifest.xml", regexp.MustCompile(`(?s)(<Identity\s+[^>]*Version=")[^"]+("[^>]*>)`), "${1}" + numeric + ".0${2}"},
		{"build/windows/nsis/wails_tools.nsh", regexp.MustCompile(`(?m)(!define INFO_PRODUCTVERSION ")[^"]+(")`), "${1}" + numeric + "${2}"},
		{"build/linux/nfpm/nfpm.yaml", regexp.MustCompile(`(?m)^(version: ")[^"]+(".*)$`), "${1}" + numeric + "${2}"},
	}
	// Prepare every update first. A missing/stale template must fail the build
	// before any file is modified or a release artifact is produced.
	type update struct {
		path string
		data []byte
		mode os.FileMode
	}
	updates := make([]update, 0, len(replacements))
	for _, r := range replacements {
		path := filepath.Join(root, filepath.FromSlash(r.path))
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !r.pattern.Match(data) {
			return fmt.Errorf("version field not found in %s", r.path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		updates = append(updates, update{path, r.pattern.ReplaceAll(data, []byte(r.value)), info.Mode().Perm()})
	}
	for _, u := range updates {
		if err := os.WriteFile(u.path, u.data, u.mode); err != nil {
			return err
		}
	}
	return nil
}
