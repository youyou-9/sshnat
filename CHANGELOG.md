# Changelog

## 1.1.3 — live tunnel status

### Fixed

- **Tunnel cards stayed on "Connecting…" after the tunnel was actually up.**
  Wails bindings hand back generated model *class instances* (`TunnelView`,
  `Host`), and Svelte 5 only proxies plain objects and arrays. The status event
  wrote `tunnel.status = "connected"` into data nothing was observing, so the
  cards, the "active tunnels" KPI, the TCP session counter and the traffic
  totals all froze on their first values — while the tunnel itself was
  forwarding fine. Tunnel and host lists are now flattened into plain objects
  on ingest, and status/stats events replace the entry instead of mutating it
  in place, so every surface updates immediately (`state-tunnels.test.ts` locks
  both guarantees).

### Changed

- Dev toolchain refresh: `undici` moved to `8.12.3` (eight high-severity
  advisories reached it through `jsdom`). The dependency gate now fails on
  moderate-and-above findings instead of any finding at all, because
  `deepmerge` — pulled in by `@sveltejs/vite-plugin-svelte` — has a
  low-severity advisory with no fixed release, and it is only used to merge
  build configuration (see `DEPENDENCY_AUDIT.md`).

## 1.1.2 — recoverable startup

### Fixed

- **Blank window on start.** A damaged or locked WebView2 `Local Storage`
  database made the module-level preference reads throw, which aborted
  evaluation of the entry module before Svelte could mount: the window stayed
  white and restarting did not help, because the damaged profile was still
  there. All storage access now goes through `lib/storage.ts`, which never
  throws, probes the backend once, and clears a store that only fails before
  `clear()`, so the UI always renders with defaults.
- **Silent startup failures.** `index.html` now carries a bootstrap watchdog:
  if the entry bundle has not mounted within four seconds, or a script/style
  fails to load or throws, the window shows a "SSHNat could not load its
  interface" panel with the captured error plus **Retry** and **Reset UI data &
  retry** (clears the damaged `localStorage`, then reloads) instead of an empty
  white page.
- **Latent boot-order bug.** `TERMINAL_FONT_MIN/MAX` and `TERMINAL_FONTS` were
  declared after the `$state` object that reads them, so a persisted terminal
  font size could hit the temporal dead zone while the module initialised. The
  constants now come first, and four bootstrap regression tests keep a
  persisted font/size and a throwing storage backend bootable.

### Changed

- The WebView2 profile now lives in `%APPDATA%\SSHNat\webview` instead of the
  binary-derived default: the path is predictable for support, and upgrading
  from 1.1.1 moves everyone onto a clean profile, so an already damaged cache
  cannot keep breaking the new build.
- The app icon is a tightly cropped rounded square; the previous artwork was
  mostly empty margin, which turned into a pale blob at 16–28 px. Sidebar,
  window/taskbar, tray, installers, and the README now share the same tile.

## 1.1.1 — brand icon everywhere

### Fixed

- The in-app sidebar brand mark now renders the real SSHNat logo instead of the
  generic terminal glyph, so the window, taskbar, tray, sidebar, and About
  dialog all show the same artwork.
- The application icon is passed to the Wails app options, which also fixes the
  About dialog / Linux window icon rather than relying on the executable
  resource alone.

### Added

- Adjustable SSH terminal font family and size in **Settings → Terminal
  appearance**, with a live preview and persisted choice.
- Terminal zoom inside the terminal: `Ctrl`/`Cmd` + mouse wheel, `Ctrl` + `+`/`-`,
  and `Ctrl` + `0` to reset. All open sessions update immediately.

## 1.1.0 — workbench, remote terminal, launch at login, update checks

### New

- **Remote terminal**: open an interactive PTY shell for any saved host from the
  host card or the command palette. Sessions follow the same ProxyJump,
  authentication, and host-key path as tunnels. Multiple sessions live in a
  docked, tabbed panel that stays mounted when collapsed, supports maximize and
  per-tab close, and is closed with the application.
- **Launch at login**: Settings toggle backed by the platform-native location on
  Windows (Run key), macOS (LaunchAgents), and Linux (XDG autostart).
- **Check for updates**: Settings compares the running version with the latest
  GitHub release using SemVer-aware ordering (release > prerelease) and offers a
  download link.
- **Workbench**: the former Dashboard and Tunnels pages are merged into a single
  workbench with a KPI strip, filters, and card/table views.
- **Command palette**: `Ctrl/Cmd+K` searches actions, hosts, and tunnels;
  `g w/h/l/s` jumps between pages.
- **Native path pickers**: private keys and known_hosts files can be chosen with
  the native file dialog instead of typing a path.
- **New Tunnel dialog**: form and "paste an ssh command" modes are now explicit
  top-level tabs; Unix socket options are collapsed under an advanced section.

### Fixes

- English UI no longer falls back to Chinese for the configuration import file
  picker (the native input button is replaced with a localized control).
- The tray menu, tray tunnel labels, and tray tooltip follow the selected
  interface language (persisted in the configuration file).
- Reconnecting tunnels show a live retry countdown.
- Persisted language selection no longer silently diverges between the UI and
  the tray; a failed backend write rolls back and reports the error.
- Terminal sessions opened during a tab close, or exited before the open call
  returned, are now closed or reported instead of leaking a session.
- `theme = system` now applies the light card styling when the OS is in light
  mode (`data-theme` is absent in that mode).

### UI/UX

- Toast notifications, confirm dialogs for destructive actions, per-card
  overflow menus, host connection status dots, keyboard focus rings, skeleton
  loading, and empty states with calls to action.
- Refined theme tokens: deeper dark surface, layered card highlights, a radial
  background glow, a tokenized scrim, and improved light-theme contrast
  (AA-compliant accent and success colors).

## 1.1.0-rc.1 — release candidate

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
