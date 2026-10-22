# Dependency audit — 2026-10-05

## Default branch alerts

The seven open GitHub Dependabot alerts at this audit date refer to `undici`
in `frontend/package-lock.json` on the default `main` branch, which locked
version `8.10.0`. All seven advisories are fixed in `8.10.2`:

- High: GHSA-w293-vg96-wgc3, GHSA-vp8m-p9jh-q5pm.
- Moderate: GHSA-pmjh-fq2x-6v4x, GHSA-2jfj-6hjv-fm6j.
- Low: GHSA-2gqq-gqf2-x968, GHSA-r53p-7pc4-xj5r, GHSA-8436-99hf-9mmv.

The release audit branch already locks `undici@8.11.2` through the development
dependency `jsdom@30.0.1`. `npm ls undici --all` confirms the installed version,
and `npm audit --json` reports zero vulnerabilities. The default branch alerts
can close once the corrected lockfile reaches that branch.

## Go fixes

A source scan of audit commit `a4fb617` with `govulncheck@v1.8.0` and the local
Go `1.26.3` toolchain found these reachable advisories:

| Component | Advisories | Repair |
| --- | --- | --- |
| `golang.org/x/crypto/ssh@v0.55.0` | GO-2026-6354, GO-2026-6355 | Upgrade `golang.org/x/crypto` to `v0.56.0` |
| Go standard library | GO-2026-6218, GO-2026-6090, GO-2026-5972, GO-2026-5856, GO-2026-5039, GO-2026-5037, GO-2026-5026 | Require Go `1.26.6` or newer (superseded by 1.26.9, see below) |

`go.mod`, all CI builds, Docker builder images, the Docker Taskfile default,
and both READMEs now use Go `1.26.9` as the minimum release baseline. Go's
automatic toolchain selection successfully downloaded and used `go1.26.9`
on Windows. Wails and `@wailsio/runtime` remain matched at `v3.0.0-beta.11`.

### 2026-10 follow-up: Go 1.26.9 required

New standard-library advisories were published after the 1.1.0-rc.1 build and
made the pinned `1.26.6` fail the reachable-vulnerability gate:

| Component | Advisories | Repair |
| --- | --- | --- |
| Go standard library (`net/http`, `crypto/tls`, `mime/multipart`) | GO-2026-6603, GO-2026-6605, GO-2026-6607, GO-2026-6608, GO-2026-6609, GO-2026-6610, GO-2026-6611, GO-2026-6612, GO-2026-6613, GO-2026-6617 | Require Go `1.26.9` or newer |

`govulncheck@v1.8.0 ./...` now reports **0 reachable vulnerabilities** with Go
`1.26.9`. The npm advisory GHSA-68fv-2mgg-jv7q (`source-map-js`) is pinned
through a `source-map-js@^1.2.2` override, so `npm audit` is clean again.

### 2026-10 follow-up: Go 1.26.10 and refreshed npm tooling

Between the 1.1.1 release and the 1.1.2 build, three more advisories landed:

| Component | Advisories | Repair |
| --- | --- | --- |
| Go standard library (`crypto/x509`) | GO-2026-6809 (distrust compromised intermediate certificates) | Require Go `1.26.10` or newer |
| `vite@8.2.2` | GHSA-698v-4r3x-3vcv, GHSA-r825-w8mm-5pxw, plus the other `launch-editor`/`server.fs` fixes in `8.2.3` | Upgrade to `vite@8.3.5` |
| `vitest@4.1.11` / `@vitest/mocker` | GHSA-qxg6-89mg-rqv8, GHSA-xp26-3454-mjgp | Upgrade to `vitest@4.1.12` |

`go.mod`, all CI builds, the Docker builder images, and both READMEs now use
Go `1.26.10`; `npm audit` reports **0 vulnerabilities** and `govulncheck`
**0 reachable vulnerabilities** for the 1.1.2 tree.

## Verification after repair

The following passed on Windows/amd64 with Go `1.26.9`:

```bash
npm --prefix frontend audit --json
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -show verbose ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -tags server ./...
go test ./...
go vet ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
```

Both Go scans report zero reachable vulnerabilities and zero vulnerabilities
in imported packages. Their remaining module-level notice, GO-2026-5932,
concerns the unmaintained `golang.org/x/crypto/openpgp` package. SSHNat does
not import this package or include it in its dependency graph; the advisory
has no patched version. This is not an affected package in SSHNat's builds.

`go build ./...` also passes. The exact Docker builder tags
`golang:1.26.6-alpine` and `golang:1.26.6-trixie` were verified with
`docker manifest inspect`; container execution is covered by the CI Docker job.
The pinned optional obfuscator `garble@v0.16.0` supports Go `1.26`, including
this baseline, and its installation guidance now names that required version.
An optional garble build with the automatically downloaded toolchain failed:
Go rejects garble's linker overlay inside the module cache. Both READMEs now
instruct users of this optional build to install Go `1.26.6` directly. The
ordinary build and security gates pass with automatic toolchain selection.

CI now runs `npm audit` and the pinned `govulncheck` source scan with the
Linux desktop build's `gtk3` tag. These checks use the live advisory databases,
so the zero-vulnerability result above is a dated result, not a guarantee for
future releases.
