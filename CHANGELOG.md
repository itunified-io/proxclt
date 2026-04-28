# Changelog

All notable changes to proxctl are documented here. Format: CalVer (`YYYY.MM.DD.TS`).

## v2026.04.28.4 — 2026-04-28

### fix: buildCreateOpts resolves storage_class → backend (#25)

`pkg/workflow/single_vm.go:buildCreateOpts` was passing the logical
`Disk.StorageClass` name (e.g. `root`, `u01`, `asm`) straight into the
Proxmox `scsi<N>=<storage>:<size>` form value, treating it as if it were a
Proxmox storage backend. Result: HTTP 500 + `{"data":null}` from the create
endpoint because Proxmox has no storage named "root".

Fix: resolve `StorageClass` against `env.Spec.StorageClasses.Resolved()` to
get the actual backend (`root → local-lvm`, `asm → nvme`, etc.). Propagate
`StorageClass.Shared` to `DiskSpec.Shared`. EFIDisk uses the same resolver.
Literal `Disk.Storage` (when set) still takes precedence — per-disk
escape hatch.

Tests in `pkg/workflow/single_vm_test.go`:
- `TestBuildCreateOpts_OVMF_StorageClass` — flipped from asserting buggy
  behavior to verifying resolution.
- `TestBuildCreateOpts_StorageClass_PropagatesShared` — new.
- `TestBuildCreateOpts_StorageClass_LiteralStorageWins` — new.

Caught while running `/lab-up --phase B` for ext3+ext4 in
itunified-io/infrastructure (plan 034) — fourth gate today after
v2026.04.28.1 (#19), .2 (#21), .3 (#23).

Closes #25.

## v2026.04.28.3 — 2026-04-28

### feat: --skip-kickstart-build flag (#23)

Adds `--skip-kickstart-build` to `workflow plan/up` and `vm create`. When
set, the workflow drops the `render-kickstart`, `build-iso`, and
`upload-iso` steps (which require a bootloader-dir + xorriso) and instead
runs:

1. `verify-kickstart-iso` — checks `<kickstart_storage>:iso/<node>_kickstart.iso` is present
2. `create-vm`
3. `start-vm`

Use this when the operator built and uploaded the kickstart ISO out-of-band
(e.g. an OEMDRV-labeled ISO that Anaconda auto-discovers, or a kickstart ISO
maintained by a separate pipeline). Avoids the bootloader-dir requirement for
operators who don't need the proxctl-built remastered install ISO.

Implementation:
- `SingleVMWorkflow.SkipKickstartBuild bool` (and `MultiNodeWorkflow.SkipKickstartBuild`)
- New `Client.StorageContentExists(ctx, node, storage, volid)` for the verify step
- New apply branch `verify-kickstart-iso`
- Plan output reflects the slimmer change set
- Tests in `pkg/workflow/single_vm_test.go::TestPlan_SkipKickstartBuild`

Caught while running `/lab-up --phase B` for ext3+ext4 in
itunified-io/infrastructure (plan 034) — the third gate after
v2026.04.28.1 (#19) and v2026.04.28.2 (#21).

Closes #23.

## v2026.04.28.2 — 2026-04-28

### fix: isNotFound recognizes Proxmox's 500+null pattern (#21)

`pkg/proxmox/vm.go:isNotFound` didn't recognize a Proxmox quirk: missing
`/nodes/<n>/qemu/<vmid>/status/current` returns HTTP 500 with body
`{"data":null}` instead of 404. As a result, `VMExists` surfaced a generic
500 error and the `workflow up` (and `vm create / start / stop / delete`)
preconditions failed for any fresh VMID:

```
Error: plan: vm-exists check: proxmox api error: status=500 message="{"data":null}"
```

Fix: when the APIError has StatusCode 500, no Errors map, and Message is
exactly `{"data":null}` (whitespace tolerated), treat it as not-found.
Other 500s (real errors) keep their semantic. Tests in
`pkg/proxmox/coverage_test.go::TestIsNotFound_500NullData` cover the new
branch + a few false-positive guards.

Caught while running `/lab-up --phase B` for ext3+ext4 in
itunified-io/infrastructure (plan 034) immediately after v2026.04.28.1
unblocked the loadEnvManifest path.

Closes #21.

## v2026.04.28.1 — 2026-04-28

### fix: kickstart/vm/workflow/boot subcommands resolve $ref envs (#19)

`internal/root/clientutil.go:loadEnvManifest()` did a raw `yaml.Unmarshal`
with no `$ref` resolution. Subcommands using it (`kickstart generate`,
`vm list/get/create/start/stop/delete`, `boot ...`, `workflow ...`) failed
with `Error: hypervisor not resolved` on any env manifest that composes via
`$ref` — the canonical pattern in `infrastructure/stacks/<stack>/env.yaml`.

The same env manifest loaded cleanly via `proxctl config render` /
`proxctl config validate`, both of which already routed through
`config.Load`. Fix is one call site: route `loadEnvManifest` through
`config.Load` so `$ref` pointers are resolved, profile extends are applied,
and secret placeholders are expanded — same path config render uses.

Tests updated: two `*_HypervisorNotResolved` tests previously locked-in the
buggy behaviour; renamed + flipped to assert the fix
(`TestVM_List_RefFixtureLoadsAndProceeds`,
`TestKickstart_Generate_RefFixtureLoaderResolvesRefs`). Stale comment on
`writeEnvFixture` updated.

Closes #19. Caught while running plan-034 `/lab-up --phase A,B,C` for the
ext3+ext4 Oracle DG lab.

## v2026.04.11.8 — 2026-04-22

### BREAKING — CLI verb rename `env` → `stack` (#15)

Aligns proxctl's bookmark terminology with the `infrastructure/stacks/`
convention. The on-disk manifest filename (`env.yaml`) and YAML kind
(`kind: Env`) are unchanged — only the CLI verb, global flag, and registry
filename were renamed.

- CLI verb: `proxctl env …` → `proxctl stack …`
- Global flag: `--env <path>` → `--stack <path>`
- Config file: `~/.proxctl/envs.yaml` → `~/.proxctl/stacks.yaml`
  (auto-renamed on first run; both paths accepted for one release)
- Hook env var: `$PROXCTL_ENV` → `$PROXCTL_STACK`

### Compatibility shims (removed in next major release)

- `proxctl env <subcommand>` still works; emits one deprecation line on
  stderr per invocation
- `--env` flag accepted as alias with a one-time stderr warning
- `$PROXCTL_ENV` promoted to `$PROXCTL_STACK` in the process environment
  with a one-time stderr warning
- `envs.yaml` is renamed in-place to `stacks.yaml` at process start; both
  filenames present triggers a warning (stacks.yaml wins)

### Tests

- New `deprecation_test.go` covers: env-verb warning, --env flag promotion,
  $PROXCTL_ENV promotion, envs.yaml migration (happy path + both-present +
  rename failure + no-op)
- Coverage: pkg/config 95.1%, pkg/workflow 96.4%, internal/root 95.2%

## v2026.04.11.7 — 2026-04-22

### Added — comprehensive documentation suite (#10)

- Full rewrite of every `docs/*.md` stub:
  - `installation.md`: Homebrew, binary, Docker, source, air-gap, shell completion, license setup, config dir, first-run verification.
  - `quick-start.md`: 8-step 5-minute walkthrough.
  - `user-guide.md`: concepts, contexts, env registry, profiles, split-file layout, VM lifecycle, kickstart, workflow orchestration, single vs multi-node, rollback, audit log.
  - `config-reference.md`: every YAML key across lab, hypervisor, networks, storage-classes, cluster, linux layers; Ref[T] model; secret resolver with pipe filters; profile inheritance; validation rules.
  - `profile-guide.md`: shipped profiles, overrides, writing custom profiles, inheritance rules.
  - `distro-guide.md`: supported distros, DistroProfile interface, adding a new distro, bootloader requirements, package sets.
  - `licensing.md`: 3-tier model, feature matrix, obtaining a license, grace period, offline activation, seat counting, pricing.
  - `troubleshooting.md`: 20 symptom → cause → fix entries.
  - `architecture.md`: component diagram, data flow, package layout, state model, concurrency, plugin points.
  - `contributing.md`: dev setup, tests, coverage, PR flow, release process, coding conventions.
- **CLI reference generator** (`cmd/docgen/main.go`) — renders the Cobra tree into `docs/cli-reference/` via `cobra/doc.GenMarkdownTree`. Makefile target `docs-cli` (alias `docs`). 52 auto-generated Markdown pages committed so GitHub renders them directly.
- **Three validated example envs** under `docs/examples/`: `host-only/`, `pg-single/`, `oracle-rac-2node/` — each a full split-file manifest that passes `proxctl config validate`.
- **README.md** rewritten: 30-second demo, key-features list, tier table, status, documentation map.

## v2026.04.11.6 — 2026-04-19

### Added — Phase 5: multi-node workflow + profile library (#8)

- **MultiNodeWorkflow** (`pkg/workflow/multi_node.go`): concurrent per-node orchestration with `golang.org/x/sync/errgroup`, fail-fast (default) + `ContinueOnError` mode, ISO upload serialization via shared `sync.Mutex` (prevents parallel upload to same storage)
- **Profile library** (`pkg/config/profiles/*.yaml`, go:embed): `oracle-rac-2node`, `pg-single`, `host-only` — extend via `extends: <name>` in lab env.yaml
- **CLI**: `workflow up/down/plan/status/verify` auto-dispatch to MultiNode when `len(Nodes) > 1`; new `workflow profile list|show <name>`
- `SingleVMWorkflow` gained optional `UploadMu *sync.Mutex` for cross-node upload coordination (minimal API addition, backward compatible)

### Concurrency verified

- `TestMultiNode_Apply_ISOUploadSerialized`: atomic counter asserts ≤1 in-flight upload at a time
- `TestMultiNode_Apply_ContinueOnError`: both node failures propagate + aggregated
- `go test -race ./...`: clean

### Coverage held

- pkg/workflow: 98% → 96.4% (new MultiNode adds ~170 statements; still >95%)
- pkg/config: 95.1% held
- internal/root: 95.2% → 95.0% held

## v2026.04.11.5 — 2026-04-19

### Tests — final coverage push (#6)

| Package | Before | After |
|---------|--------|-------|
| pkg/license | 72.7% | **100.0%** |
| internal/root | 19.2% | **95.2%** |
| **Total** | 75.6% | **96.2%** |

- 116 new tests (4 license + 112 CLI handler)
- Minor refactor: `osExit` package-level var for testable `Execute()` error branch
- Cobra test harness with fresh `NewRootCmd` per test; httptest Proxmox injected via env vars; `$HOME` isolated via `t.Setenv`
- All substantial packages now ≥95%; `cmd/proxctl` (main.go) and `pkg/state` (scaffold stub) remain at 0% — not fixable without real work

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
