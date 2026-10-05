package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const MaxConfigBytes = 10 << 20

// ParseSettings accepts exactly one configuration document. Unknown fields
// fail loudly so a typo or a backup from a newer schema cannot lose options.
func ParseSettings(data []byte) (*Settings, error) {
	settings, err := decodeSettings(data)
	if err != nil {
		return nil, err
	}
	if err := Validate(settings); err != nil {
		return nil, err
	}
	return settings, nil
}

// Decode storage without discarding unknown fields. Semantic validation is
// separate so an existing configuration can still be repaired in the editor.
func decodeSettings(data []byte) (*Settings, error) {
	if len(data) > MaxConfigBytes {
		return nil, errors.New("config exceeds 10 MiB")
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, errors.New("config must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var settings Settings
	if err := decoder.Decode(&settings); err != nil {
		return nil, fmt.Errorf("config: decode: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("config must contain one JSON document")
	}
	if settings.Version == 0 {
		settings.Version = 1
	}
	if settings.Version != 1 {
		return nil, errors.New("unsupported config version (expected 1)")
	}
	return &settings, nil
}

// Validate checks references as well as individual values before importing
// or launching a configuration. Missing credentials are allowed: a redacted
// export remains a usable template that can be completed in the host editor.
func Validate(settings *Settings) error {
	if settings == nil || settings.Version != 1 {
		return errors.New("unsupported config version (expected 1)")
	}
	hosts := make(map[string]Host, len(settings.Hosts))
	for _, host := range settings.Hosts {
		if strings.TrimSpace(host.ID) == "" {
			return errors.New("host id is required")
		}
		if _, exists := hosts[host.ID]; exists {
			return fmt.Errorf("duplicate host id %q", host.ID)
		}
		if err := ValidateHost(host); err != nil {
			return fmt.Errorf("host %q: %w", host.ID, err)
		}
		hosts[host.ID] = host
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("jump host cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		host, exists := hosts[id]
		if !exists {
			return fmt.Errorf("jump host %q not found", id)
		}
		visiting[id] = true
		for _, jump := range host.JumpHostIDs {
			if strings.TrimSpace(jump) == "" {
				return fmt.Errorf("host %q has an empty jump host reference", id)
			}
			if err := visit(jump); err != nil {
				return err
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for id := range hosts {
		if err := visit(id); err != nil {
			return err
		}
	}
	tunnelIDs := map[string]bool{}
	for _, tunnel := range settings.Tunnels {
		if strings.TrimSpace(tunnel.ID) == "" || tunnelIDs[tunnel.ID] {
			return fmt.Errorf("empty or duplicate tunnel id %q", tunnel.ID)
		}
		tunnelIDs[tunnel.ID] = true
		if _, exists := hosts[tunnel.HostID]; !exists {
			return fmt.Errorf("tunnel %q references missing host %q", tunnel.ID, tunnel.HostID)
		}
		if err := ValidateTunnel(tunnel); err != nil {
			return fmt.Errorf("tunnel %q: %w", tunnel.ID, err)
		}
	}
	return nil
}

func ValidateHost(host Host) error {
	if strings.TrimSpace(host.Name) == "" || strings.TrimSpace(host.Host) == "" || strings.TrimSpace(host.User) == "" {
		return errors.New("host name, address and user are required")
	}
	if strings.ContainsAny(host.Host, " \t\r\n\x00/\\") {
		return errors.New("host address must be a hostname or IP without a port")
	}
	if host.Port != 0 && !validPort(host.Port) {
		return errors.New("host port must be between 1 and 65535")
	}
	if host.KeepaliveSeconds < -1 || host.KeepaliveSeconds > 86400 {
		return errors.New("keepalive must be -1 (disabled), 0 (default), or 1..86400 seconds")
	}
	if host.ConnectTimeoutSeconds < 0 || host.ConnectTimeoutSeconds > 300 {
		return errors.New("connect timeout must be 0 (default) or 1..300 seconds")
	}
	switch host.HostKeyPolicy {
	case "", HostKeyAcceptNew, HostKeyStrict:
	default:
		return errors.New("host key policy must be accept-new or strict")
	}
	if strings.ContainsRune(host.KnownHostsFile, '\x00') {
		return errors.New("known_hosts path must not contain a NUL")
	}
	switch host.Auth.Method {
	case "", AuthMethodPassword, AuthMethodAgent:
	case AuthMethodKey:
		if strings.TrimSpace(host.Auth.KeyPath) == "" {
			return errors.New("key authentication requires a key path")
		}
	default:
		return fmt.Errorf("unknown auth method %q", host.Auth.Method)
	}
	return nil
}

func ValidateTunnel(tunnel Tunnel) error {
	if strings.TrimSpace(tunnel.Name) == "" || strings.TrimSpace(tunnel.HostID) == "" {
		return errors.New("tunnel name and host are required")
	}
	for _, path := range []string{tunnel.LocalSocket, tunnel.RemoteSocket, tunnel.TargetSocket} {
		if path != "" && (strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00')) {
			return errors.New("socket path must not be blank or contain a NUL")
		}
	}
	switch tunnel.Type {
	case TypeLocal, TypeRemote:
		port, socket := tunnel.LocalPort, tunnel.LocalSocket
		if tunnel.Type == TypeRemote {
			port, socket = tunnel.RemotePort, tunnel.RemoteSocket
		}
		if socket != "" && port != 0 {
			return errors.New("listener port and socket are mutually exclusive")
		}
		if socket == "" && !validPort(port) && !(tunnel.Type == TypeRemote && port == 0) {
			return errors.New("listener port must be between 1 and 65535")
		}
		if tunnel.TargetSocket != "" && (tunnel.TargetHost != "" || tunnel.TargetPort != 0) {
			return errors.New("target host/port and socket are mutually exclusive")
		}
		if tunnel.TargetSocket == "" && !validPort(tunnel.TargetPort) {
			return errors.New("target port must be between 1 and 65535")
		}
	case TypeDynamic:
		if !validPort(tunnel.SocksPort) {
			return errors.New("SOCKS port must be between 1 and 65535")
		}
	default:
		return fmt.Errorf("unknown tunnel type %q", tunnel.Type)
	}
	return nil
}
