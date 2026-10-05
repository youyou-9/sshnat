package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sshnat/sshnat/core/config"
)

type ConfigImportResult struct {
	HostsAdded   int    `json:"hostsAdded"`
	TunnelsAdded int    `json:"tunnelsAdded"`
	BackupPath   string `json:"backupPath,omitempty"`
}

// Export returns portable JSON. Credentials are omitted unless the user
// explicitly requests a full backup; exporting never modifies stored data.
func (st *SettingsService) Export(includeSecrets bool) (string, error) {
	st.s.tmu.Lock()
	defer st.s.tmu.Unlock()
	settings, err := st.s.Store.Load()
	if err != nil {
		return "", err
	}
	if !includeSecrets {
		for i := range settings.Hosts {
			settings.Hosts[i].Auth.Password = ""
			settings.Hosts[i].Auth.KeyPassphrase = ""
		}
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	return string(data), err
}

// ExportFile opens the native save dialog and writes a private atomic backup.
// An empty result means that the user cancelled the dialog.
func (st *SettingsService) ExportFile(includeSecrets bool) (string, error) {
	st.s.emu.RLock()
	choose := st.s.chooseExport
	st.s.emu.RUnlock()
	if choose == nil {
		return "", errors.New("native save dialog is unavailable")
	}
	path, err := choose()
	if err != nil || path == "" {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	active, err := filepath.Abs(st.s.Store.Path())
	if err != nil {
		return "", err
	}
	if strings.EqualFold(absolute, active) {
		return "", errors.New("choose a backup file separate from the active configuration")
	}
	// Resolve existing file identity as well as its spelling. A junction or
	// symlinked directory may otherwise make a backup overwrite active config.
	if backupInfo, err := os.Stat(absolute); err == nil {
		if activeInfo, err := os.Stat(active); err == nil && os.SameFile(backupInfo, activeInfo) {
			return "", errors.New("choose a backup file separate from the active configuration")
		}
	}
	document, err := st.Export(includeSecrets)
	if err != nil {
		return "", err
	}
	var settings config.Settings
	if err := json.Unmarshal([]byte(document), &settings); err != nil {
		return "", err
	}
	if err := config.NewStore(absolute).Save(&settings); err != nil {
		return "", err
	}
	return absolute, nil
}

// Import validates a complete document before mutation. Merge appends with
// remapped IDs, so existing tunnels and host credentials are never overwritten.
// Replace requires stopped tunnels and creates a full backup first.
func (st *SettingsService) Import(document, mode string) (*ConfigImportResult, error) {
	if mode != "merge" && mode != "replace" {
		return nil, errors.New("import mode must be merge or replace")
	}
	incoming, err := config.ParseSettings([]byte(document))
	if err != nil {
		return nil, err
	}
	st.s.tmu.Lock()
	changed := false
	defer func() {
		st.s.tmu.Unlock()
		if changed {
			st.s.emitConfigChanged()
		}
	}()
	current, err := st.s.Store.Load()
	if err != nil {
		return nil, err
	}
	result := &ConfigImportResult{HostsAdded: len(incoming.Hosts), TunnelsAdded: len(incoming.Tunnels)}
	if mode == "replace" {
		for _, tunnel := range current.Tunnels {
			if st.s.Sup.HasActiveRun(tunnel.ID) {
				return nil, errors.New("stop all tunnels before replacing configuration")
			}
		}
		backup, err := saveBackup(st.s.Store)
		if err != nil {
			return nil, err
		}
		result.BackupPath = backup
	} else {
		ids := make(map[string]string, len(incoming.Hosts))
		for i := range incoming.Hosts {
			old := incoming.Hosts[i].ID
			incoming.Hosts[i].ID = config.NewID("host")
			ids[old] = incoming.Hosts[i].ID
		}
		for i := range incoming.Hosts {
			for j, id := range incoming.Hosts[i].JumpHostIDs {
				incoming.Hosts[i].JumpHostIDs[j] = ids[id]
			}
		}
		for i := range incoming.Tunnels {
			incoming.Tunnels[i].ID = config.NewID("tnl")
			incoming.Tunnels[i].HostID = ids[incoming.Tunnels[i].HostID]
		}
		current.Hosts = append(current.Hosts, incoming.Hosts...)
		current.Tunnels = append(current.Tunnels, incoming.Tunnels...)
		incoming = current
	}
	if err := config.Validate(incoming); err != nil {
		return nil, err
	}
	if err := st.s.Store.Save(incoming); err != nil {
		return nil, err
	}
	changed = true
	if mode == "replace" {
		for _, tunnel := range current.Tunnels {
			st.s.Stats.Delete(tunnel.ID)
		}
	}
	return result, nil
}

// saveBackup preserves the exact previous document, including credentials,
// in the configuration directory. CreateTemp guarantees no backup overwrite.
func saveBackup(store *config.Store) (string, error) {
	data, err := os.ReadFile(store.Path())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read config for backup: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(store.Path()), "config-backup-*.json")
	if err != nil {
		return "", fmt.Errorf("create config backup: %w", err)
	}
	path := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}
