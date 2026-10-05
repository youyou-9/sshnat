# Changelog

## 1.1.0 — release candidate

### New controls

- Test a host's SSH connection before starting a tunnel.
- Edit ordered ProxyJump chains, connection timeouts, host key policies,
  known-hosts paths, keepalive intervals, and listener bind addresses.
- Export redacted configurations or complete backups; import by merging or
  replacing with a backup of the previous configuration.
- Copy and import SSH commands for POSIX shells or PowerShell.
- Run a separate headless daemon container; validate configurations with
  `sshnatd -check` and inspect the installed version with `-version`.

### Fixes

- Cancel blocked SSH connection/listener requests and clean up every jump,
  listener, agent connection, and owned Unix socket.
- Preserve responses after a client half-closes and count both traffic
  directions correctly.
- Detect connection loss when keepalive is disabled and prevent overlapping
  tunnel runs after a stop timeout.
- Serialize configuration changes with tunnel startup; reject invalid JSON,
  broken host references, unsupported schemas, and ProxyJump cycles.
- Preserve IPv6, Unix socket routes, paths with spaces, and advanced host
  settings during supported SSH command import/export.
- Refresh tray entries after configuration changes, allow stopping retrying
  tunnels, and clean up tunnels during native application shutdown.
- Improve per-tunnel pending feedback, batch errors, copy feedback, accessible
  controls, and action visibility at the minimum window size.
- Use the SSHNat arrow icon for native desktop and tray assets; treat native
  save-dialog cancellation as a cancelled action instead of an export failure.
- Align Linux package dependencies with the GTK renderer and preserve Unix
  executable permissions in packages.

### Security and release tooling

- Upgrade Go to 1.26.6 and `golang.org/x/crypto` to v0.56.0 to fix reachable
  standard-library and SSH vulnerabilities found in the audit.
- Update frontend dependencies and add npm audit and govulncheck CI gates.
- Synchronize display/native versions; verify three desktop builds, six daemon
  builds, Docker startup, archive contents, executable bits, and SHA256 sums.

### Migration and validation

Existing version-1 configurations remain supported. Back up credentials before
replacing a configuration. A redacted export omits passwords and key passphrases;
private key files and known-hosts files must be transferred separately.

See [AUDIT_REPORT.md](AUDIT_REPORT.md) for executed checks and
[RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) for platform verification. The GitHub
Release is published by the `v1.1.0-rc.1` tag workflow and is marked as a prerelease.
