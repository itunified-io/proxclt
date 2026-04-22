# Changelog

All notable changes to proxctl are documented here. Format: CalVer (`YYYY.MM.DD.TS`).

## v2026.04.11.4 — 2026-04-19

### Tests — coverage hardening to ≥95% (#4)

| Package | Before | After | Delta |
|---------|--------|-------|-------|
| pkg/config | 81.6% | **95.0%** | +13.4pp |
| pkg/proxmox | 83.8% | **97.5%** | +13.7pp |
| pkg/kickstart | 69.5% | **97.6%** | +28.1pp |
| pkg/workflow | 53.7% | **98.0%** | +44.3pp |

- ~127 new tests across 4 packages; no public API changes
- Minor testability refactor in `pkg/kickstart`: extracted `newRendererFromFS(fs.FS)` for injectable test FS (public `NewRenderer()` unchanged)
- All error paths + Rollback + HTTP error envelopes + CIDR/resolver/validator edge cases covered
- Residual uncovered lines are genuinely unreachable via public API (defensive nil guards, stdlib error returns on happy-path inputs)
- `pkg/license` (72.7%) and `internal/root` (19%) remain below 95% — CLI wiring tests are Phase 5 scope (integration tests)

## v2026.04.11.3 — 2026-04-19

### Changed (BREAKING)
- Renamed tool from `proxclt` → `proxctl` (per user request, ADR follow-up pending)
- Go module path: `github.com/itunified-io/proxctl`
- Binary: `proxctl`
- Config/state dir: `~/.proxctl/`
- Env vars: `PROXCTL_*`
- Repo: `itunified-io/proxctl` (GitHub auto-redirects from old URL)

## v2026.04.11.2 — 2026-04-19

### Added — Phase 2 core implementation

- `pkg/config` (81.6% coverage): full struct models with validator tags, YAML unmarshal, $ref loader + profile extends, secret resolver (env/file/vault/gen/ref + base64/default pipes), cross-field validators (unique vm_ids/hostnames/IPs, RAC invariants, network + storage_class refs), JSON Schema export (#1)
- `pkg/proxmox` (83.8% coverage): core REST client with token auth + task polling, VM CRUD + list + status, storage list + ISO upload, snapshot CRUD + rollback, first-boot ISO attach + SetBootOrder + ConfigureFirstBoot (#1)
- `pkg/kickstart` (69.5% coverage): Go text/template + sprig renderer, templates for OL8/OL9/Ubuntu 22.04 (base + common partials), xorriso/mkisofs ISO builder (#1)
- `pkg/workflow` (53.7% coverage): SingleVMWorkflow with Plan → Apply → Verify → Rollback + Up/Down helpers, dry-run support (#1)
- `internal/root/clientutil.go`: Proxmox client loader from `~/.proxctl/config.yaml` (kubectl-style contexts) with env-var fallback
- CLI handlers wired for real: `config validate|render|schema`, `vm create|start|stop|reboot|delete|list|status`, `snapshot create|restore|list|delete`, `kickstart generate|build-iso|upload|distros`, `boot configure-first-boot|eject-iso`, `workflow plan|up|down|status|verify`

### Verified

- `go build ./...`, `go vet ./...`, `staticcheck ./...`, `go test ./...` all clean
- `proxctl config validate <env.yaml>` roundtrip green
- `proxctl kickstart distros` lists 3 supported distros

## v2026.04.11.1 — 2026-04-22

### Added

- Initial scaffold from Phase 1 of the proxctl + linuxctl plan
  (`itunified-io/infrastructure` docs/plans/024-proxctl-design.md).
- Go module `github.com/itunified-io/proxctl` on Go 1.23.
- Cobra command tree covering 9 subcommand groups:
  `config`, `env`, `vm`, `snapshot`, `kickstart`, `boot`, `workflow`, `license`, `version`.
  Every leaf subcommand returns `not implemented yet (scaffold)` — real logic lands in Phase 2+.
- `pkg/license` — `ToolTier` constants + `ToolCatalog` map + stub `Check()`.
- `pkg/state` — SQLite handle stub for Phase 2 `modernc.org/sqlite` integration.
- `pkg/config` — `Env`, `Hypervisor`, `Networks`, `StorageClasses`, `Cluster` structs;
  `ParsePlaceholders` regex for `${vault,env,file,gen,ref:…}` syntax.
- `pkg/version` — ldflags-injected `Version` / `Commit` / `Date`.
- AGPL-3.0 LICENSE, README, Makefile, .goreleaser.yaml, Dockerfile, CI workflow.
- Unit tests: root command + version + license gate + config validate + placeholder parser.

Ref: itunified-io/infrastructure#389
