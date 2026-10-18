package app

import (
	"path/filepath"
	"testing"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
)

func newTestServices(t *testing.T) *Services {
	t.Helper()
	store := config.NewStore(filepath.Join(t.TempDir(), "config.json"))
	return NewServices(store, supervisor.New(store, stats.NewRegistry()), stats.NewRegistry())
}

func TestLanguageRoundTrip(t *testing.T) {
	s := newTestServices(t)
	defer s.Term.CloseAll()

	lang, err := s.SettingsService().GetLanguage()
	if err != nil || lang != "" {
		t.Fatalf("initial language = %q, %v; want empty", lang, err)
	}
	if err := s.SettingsService().SetLanguage("en"); err != nil {
		t.Fatalf("SetLanguage: %v", err)
	}
	lang, err = s.SettingsService().GetLanguage()
	if err != nil || lang != "en" {
		t.Fatalf("language = %q, %v; want en", lang, err)
	}
	if err := s.SettingsService().SetLanguage("fr"); err == nil {
		t.Fatal("SetLanguage accepted unsupported language")
	}
}

func TestTerminalOpenRejectsUnknownHost(t *testing.T) {
	s := newTestServices(t)
	defer s.Term.CloseAll()

	if _, err := s.TerminalService().Open(OpenTerminalRequest{HostID: "missing", Cols: 80, Rows: 24}); err == nil {
		t.Fatal("Open accepted unknown host")
	}
	if _, err := s.TerminalService().Open(OpenTerminalRequest{Cols: 80, Rows: 24}); err == nil {
		t.Fatal("Open accepted empty host id")
	}
}

func TestTerminalCloseIsIdempotent(t *testing.T) {
	s := newTestServices(t)
	defer s.Term.CloseAll()

	if err := s.TerminalService().Close("term_never_opened"); err != nil {
		t.Fatalf("Close of unknown session should be a no-op, got %v", err)
	}
}
