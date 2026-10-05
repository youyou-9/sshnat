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
| Go standard library | GO-2026-6218, GO-2026-6090, GO-2026-5972, GO-2026-5856, GO-2026-5039, GO-2026-5037, GO-2026-5026 | Require Go `1.26.6` or newer |

`go.mod`, all CI builds, Docker builder images, the Docker Taskfile default,
and both READMEs now use Go `1.26.6` as the minimum release baseline. Go's
automatic toolchain selection successfully downloaded and used `go1.26.6`
on Windows. Wails and `@wailsio/runtime` remain matched at `v3.0.0-beta.11`.

## Verification after repair

The following passed on Windows/amd64 with Go `1.26.6`:

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
