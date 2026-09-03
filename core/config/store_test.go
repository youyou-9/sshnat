package config

import (
	"path/filepath"
	"sync"
	"testing"
)

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
