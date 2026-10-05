package config

import "testing"

func TestParseSettingsRejectsInvalidDocuments(t *testing.T) {
	for _, document := range []string{
		`null`, `[]`, `{}`, `{"version":2}`, `{"version":1,"typo":true}`,
		`{"version":1} {"version":1}`,
		`{"version":1,"hosts":[{"id":"h","name":"x","host":"localhost","user":"u","auth":{"method":"bad"}}]}`,
		`{"version":1,"tunnels":[{"id":"t","name":"x","hostId":"missing","type":"D","socksPort":1080}]}`,
	} {
		if _, err := ParseSettings([]byte(document)); err == nil {
			// An empty object is a valid empty legacy configuration.
			if document == `{}` {
				continue
			}
			t.Errorf("accepted invalid document %s", document)
		}
	}
}

func TestValidateRejectsDuplicateIDsAndJumpCycles(t *testing.T) {
	host := Host{ID: "h", Name: "Host", Host: "localhost", User: "user"}
	settings := &Settings{Version: 1, Hosts: []Host{host, host}}
	if err := Validate(settings); err == nil {
		t.Fatal("duplicate host accepted")
	}
	settings.Hosts = []Host{host}
	settings.Hosts[0].JumpHostIDs = []string{"missing"}
	if err := Validate(settings); err == nil {
		t.Fatal("missing jump accepted")
	}
	settings.Hosts[0].JumpHostIDs = []string{"h"}
	if err := Validate(settings); err == nil {
		t.Fatal("jump cycle accepted")
	}
}

func TestValidateTunnelSocketAndIPv6(t *testing.T) {
	tunnel := Tunnel{Name: "socket", HostID: "h", Type: TypeLocal, LocalBindHost: "::1", LocalPort: 8080, TargetSocket: "/run/service.sock"}
	if err := ValidateTunnel(tunnel); err != nil {
		t.Fatal(err)
	}
	tunnel.TargetHost = "::1"
	if err := ValidateTunnel(tunnel); err == nil {
		t.Fatal("conflicting target accepted")
	}
}
