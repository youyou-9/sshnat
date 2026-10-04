package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var metadataFixtures = map[string]string{
	"build/config.yml":                    `  version: "1.0.0" # release version`,
	"build/darwin/Info.plist":             `<key>CFBundleVersion</key><string>1.0.0</string><key>CFBundleShortVersionString</key><string>1.0.0</string>`,
	"build/darwin/Info.dev.plist":         `<key>CFBundleVersion</key><string>1.0.0</string><key>CFBundleShortVersionString</key><string>1.0.0</string>`,
	"build/windows/info.json":             `{"fixed":{"file_version":"1.0.0"},"info":{"0409":{"FileVersion":"1.0.0","ProductVersion":"1.0.0"}}}`,
	"build/windows/wails.exe.manifest":    `<assemblyIdentity type="win32" name="com.sshnat.app" version="1.0.0.0" processorArchitecture="*"/>`,
	"build/windows/msix/app_manifest.xml": `<Identity Name="com.sshnat.app" Version="1.0.0.0" ProcessorArchitecture="x64" />`,
	"build/windows/nsis/wails_tools.nsh":  `!define INFO_PRODUCTVERSION "1.0.0"`,
	"build/linux/nfpm/nfpm.yaml":          `version: "1.0.0"`,
}

func seedMetadata(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for rel, contents := range metadataFixtures {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSyncMetadataPropagatesReleaseTag(t *testing.T) {
	root := seedMetadata(t)
	if err := syncMetadata(root, "v2.3.4-rc.1"); err != nil {
		t.Fatal(err)
	}
	for rel := range metadataFixtures {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "1.0.0") || !strings.Contains(string(data), "2.3.4") {
			t.Fatalf("%s did not receive release version: %s", rel, data)
		}
		if strings.Contains(string(data), "rc.1") {
			t.Fatalf("%s contains a prerelease suffix in a native version field", rel)
		}
	}
}

func TestSyncMetadataRejectsInvalidVersionWithoutWriting(t *testing.T) {
	for _, invalid := range []string{"", "v2", "latest", "1.2.3; echo bad", "99999.0.0"} {
		t.Run(invalid, func(t *testing.T) {
			root := seedMetadata(t)
			if err := syncMetadata(root, invalid); err == nil {
				t.Fatal("expected invalid version error")
			}
			for rel, want := range metadataFixtures {
				data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
				if string(data) != want {
					t.Fatalf("%s changed after invalid version", rel)
				}
			}
		})
	}
}

func TestSyncMetadataValidatesEveryTemplateBeforeWriting(t *testing.T) {
	root := seedMetadata(t)
	if err := os.WriteFile(filepath.Join(root, "build/config.yml"), []byte("missing version field"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncMetadata(root, "2.3.4"); err == nil {
		t.Fatal("expected missing template field error")
	}
	for rel, want := range metadataFixtures {
		if rel == "build/config.yml" {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if string(data) != want {
			t.Fatalf("%s changed after incomplete template", rel)
		}
	}
}
