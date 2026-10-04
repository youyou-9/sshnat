package app

import (
	"errors"
	"os"
	"testing"
)

func TestSettingsExportFileFailureLeavesActiveConfigurationUntouched(t *testing.T) {
	for _, scenario := range []string{"unavailable dialog", "dialog error", "write error"} {
		t.Run(scenario, func(t *testing.T) {
			s := setupTestServices(t)
			if err := s.Store.Save(importFixture()); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(s.Store.Path())
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "dialog error":
				s.SetExportChooser(func() (string, error) { return "", errors.New("dialog failed") })
			case "write error":
				directory := t.TempDir()
				s.SetExportChooser(func() (string, error) { return directory, nil })
			}
			if _, err := s.SettingsService().ExportFile(false); err == nil {
				t.Fatal("failed export did not return an error")
			}
			after, err := os.ReadFile(s.Store.Path())
			if err != nil || string(before) != string(after) {
				t.Fatal("failed export changed active configuration")
			}
		})
	}
}
