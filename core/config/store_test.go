package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreLoadProtectsUnsupportedDocuments(t *testing.T) {
	for _, document := range []string{
		`null`, `[]`, `{"version":2}`, `{"version":1,"futureOption":true}`,
		`{"version":1,"hosts":[{"id":"h","auth":{"futureOption":true}}]}`,
		`{"version":1} {"version":1}`,
	} {
		t.Run(document, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewStore(path).Load(); err == nil {
				t.Fatal("unsupported config could be loaded and overwritten by an edit")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != document {
				t.Fatal("loading changed the original document")
			}
		})
	}
}

func TestStoreLoadLegacyAndRepairableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// Retain legacy version defaults and let the host editor repair missing
	// fields. Strict import and daemon checks perform semantic validation.
	if err := os.WriteFile(path, []byte(`{"hosts":[{"id":"h","name":"repair-me"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := NewStore(path).Load()
	if err != nil || settings.Version != 1 || len(settings.Hosts) != 1 {
		t.Fatalf("legacy config failed to load: %v", err)
	}
	if err := Validate(settings); err == nil {
		t.Fatal("repairable config unexpectedly passed semantic validation")
	}
}

func TestStoreAtomicSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	store := NewStore(cfgPath)

	st := &Settings{
		Version: 1,
		Hosts: []Host{
			{ID: "host-1", Name: "server-1", Host: "1.2.3.4", Port: 22, User: "root"},
		},
		Tunnels: []Tunnel{
			{ID: "tnl-1", Name: "db", HostID: "host-1", Type: TypeLocal, LocalPort: 8080, TargetHost: "127.0.0.1", TargetPort: 3306},
		},
	}

	if err := store.Save(st); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded.Hosts) != 1 || loaded.Hosts[0].ID != "host-1" {
		t.Fatalf("unexpected hosts: %+v", loaded.Hosts)
	}
	if len(loaded.Tunnels) != 1 || loaded.Tunnels[0].ID != "tnl-1" {
		t.Fatalf("unexpected tunnels: %+v", loaded.Tunnels)
	}

	// Saving an existing configuration is the common edit path. This also
	// guards the Windows replacement implementation, where os.Rename cannot
	// overwrite an existing destination.
	st.Hosts[0].Name = "server-1-updated"
	if err := store.Save(st); err != nil {
		t.Fatalf("replace existing config: %v", err)
	}
	loaded, err = store.Load()
	if err != nil || loaded.Hosts[0].Name != "server-1-updated" {
		t.Fatalf("updated config was not persisted: %+v err=%v", loaded, err)
	}
}

func TestStoreConcurrentSaves(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	store := NewStore(cfgPath)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			st := &Settings{
				Version: 1,
				Hosts: []Host{
					{ID: NewID("host"), Name: "server", Host: "1.2.3.4", Port: 22, User: "root"},
				},
			}
			_ = store.Save(st)
		}(i)
	}
	wg.Wait()

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load after concurrent saves failed: %v", err)
	}
	if loaded == nil || loaded.Version != 1 {
		t.Fatalf("corrupted settings: %+v", loaded)
	}
}
