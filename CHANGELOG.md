# Changelog

All notable changes to proxctl are documented here. Format: CalVer (`YYYY.MM.DD.TS`).

## v2026.04.30.7 — 2026-04-30

### fix: ubuntu2404 autoinstall must inject BEFORE casper `---` separator

v2026.04.30.5/6 patched the GRUB linux line by appending the autoinstall
args at the end. Caught during the dbx-control live deployment: that placed
`autoinstall` AFTER `---`, where casper reserves args for the post-install
kernel and the live boot's Subiquity does NOT see them. Result: Subiquity
defaulted to interactive mode and prompted the operator to confirm —
exactly the failure mode the patch was meant to eliminate.

This release:

- `pkg/kickstart/ubuntu_iso_builder.go` — `patchKernelCmdline` now uses
  `injectBeforeSeparator()` which inserts immediately before the casper
  `---` separator (or appends if no separator exists).
- Default `AutoinstallKernelArgs` updated to `s=/cdrom/cidata/` (live
  system mounts the install ISO at `/cdrom`, not `/`).
- New unit test `TestPatchKernelCmdline_InjectsBeforeCasperSeparator`
  asserting `autoinstall` index < `---` index.
- Existing tests updated for the new path + injection position.

Direct fix for VM 2900 / dbx-control deployment (infrastructure ADR-0109).

## v2026.04.30.6 — 2026-04-30

### feat: `proxctl kickstart build-ubuntu` CLI + macaddress NIC matching

- `proxctl kickstart build-ubuntu --env <env.yaml> --node <name> --source-iso <path>`
  drives the v2026.04.30.5 `UbuntuISOBuilder` end-to-end: renders
  `user-data.tmpl` + `meta-data.tmpl` from the env manifest, writes them to a
  scratch cidata dir, calls the builder to produce a remastered Ubuntu install
  ISO with autoinstall in GRUB + cidata at `/cidata/`. Operator uploads the
  result via `proxctl kickstart upload`.
- `Renderer.RenderTemplate(env, nodeName, templateName)` — new public method
  for the multi-file Subiquity flow (Render() picks a single entry point;
  Subiquity needs both user-data + meta-data rendered separately).
- `pkg/kickstart/templates/ubuntu2404/user-data.tmpl` — netplan ethernet match
  is now `macaddress` (when manifest has a real MAC) with fallback to
  `name: "en*"` (when MAC is `auto` or empty), plus `set-name` to rename the
  matched interface back to the manifest-declared logical name. Fixes a class
  of failures where Ubuntu 24.04 names virtio NICs `enp6s18` on q35 vs
  `ens18` on i440fx — static names don't survive bus changes.

Workflow dispatcher integration (`SingleVMWorkflow` auto-routes ubuntu2404 to
the new builder + uploads the remastered ISO as the install ISO instead of
the upstream one) ships in v2026.04.30.7. For now operators using
`distro: ubuntu2404` build the ISO out-of-band via the new CLI, upload it,
and run `proxctl-stack-up --skip-kickstart-build` (or equivalent) against a
manifest that points `iso.image` at the remastered ISO.

## v2026.04.30.5 — 2026-04-30

### feat: ubuntu2404 Subiquity autoinstall via cidata + GRUB cmdline injection

Ubuntu 22.04.5+ / 24.04 live-server ISOs ship Subiquity (autoinstall via
cloud-init NoCloud datasource) — no isolinux, no preseed support. Without a
kernel-cmdline `autoinstall ds=nocloud\;s=/cidata/`, Subiquity prompts the
operator to confirm, breaking unattended provisioning. The legacy
`ubuntu2204` preseed template doesn't apply.

This release adds:

- `pkg/kickstart/templates/ubuntu2404/user-data.tmpl` — Subiquity autoinstall
  template with `interactive-sections: []` (truly unattended), distro-aware
  Wheel→sudo mapping in late-commands (`Wheel: true` renders `usermod -aG
  sudo <user>` + sudoers.d entry, since Ubuntu has no `wheel` group), SSH
  authorized-keys for primary user + root, hostname/keyboard/locale/network
  rendered from manifest.
- `pkg/kickstart/templates/ubuntu2404/meta-data.tmpl` — minimal cloud-init
  meta-data with instance-id keyed on hostname.
- `pkg/kickstart/ubuntu_iso_builder.go` — repacks the upstream Ubuntu install
  ISO via xorriso `-indev`/`-outdev` (no 3 GB extraction overhead),
  preserving original El Torito + UEFI boot images via `-boot_image any
  replay`. Patches every GRUB `linux`/`linuxefi` line + isolinux `append`
  line to append `autoinstall ds=nocloud\;s=/cidata/`. Idempotent (skips if
  `autoinstall` already present). Embeds rendered `user-data` + `meta-data`
  at `/cidata/` in the ISO root.
- `pkg/kickstart/renderer.go` — `pickEntry()` recognizes `user-data.tmpl` as
  a third entry-point in addition to `base.ks.tmpl` (RHEL/OEL kickstart) and
  `preseed.cfg.tmpl` (legacy Debian preseed).
- `pkg/config/hypervisor.go` — `KickstartConfig.Distro` accepts `ubuntu2404`.
- 6 unit tests in `pkg/kickstart/ubuntu_iso_builder_test.go` covering GRUB
  linux line injection, idempotency, isolinux append, linuxefi variant,
  defaults, and missing-file rejection.

Direct fix for the dbx-control deployment (infrastructure ADR-0109): VM 2900
hung on Subiquity confirm because the legacy ubuntu2204 preseed builder
couldn't generate a valid Subiquity-aware install ISO. Operator demand:
"not good workaround, enhance kickstart auto install for ubuntu" — this
ships the proper engineering fix.

Workflow integration (single_vm dispatcher) lands in v2026.04.30.6+; this
release ships the builder + templates + renderer wiring + tests.

## v2026.04.30.4 — 2026-04-30

### feat: replay + golden traces (#46)

Item 2 of itunified-io/infrastructure agentic-AI hardening roadmap (Wave B foundation; ADR-0101 in infrastructure repo).

- `pkg/trace/schema.go` — Record struct with Plan-RAG step_id, tool, input/output, decision, hash chain (SHA-256 per ADR-0095). SchemaVersion 1.
- `pkg/trace/writer.go` — append-only JSONL writer; resumes hash chain when reopening an existing file
- `pkg/trace/reader.go` — line-by-line reader; LoadAll convenience reads + verifies chain
- `pkg/trace/replay.go` — three modes:
  - **ModeStrict**: re-execute, abort on first output divergence (CI gate)
  - **ModeObserve**: re-execute, log divergence, never abort (audit / dashboard)
  - **ModeDryRun**: validate chain + print actions, no execution
- `internal/root/replay.go` — new `proxctl replay <trace.jsonl>` subcommand with `--observe` / `--dry-run` / `--verbose` flags
- 12 unit tests in `pkg/trace/{schema,replay}_test.go`

Default executor (live tool dispatch via Bash/MCP/CLI) ships in v2026.04.30.5+; this release ships the trace primitive + replay engine + CLI wiring. The current default executor returns recorded output verbatim (so a fresh replay against an unchanged trace passes — useful for chain verification + CI structural validation).

Used by linuxctl mirror (issue tbd) + infrastructure `lab-replay.sh` wrapper (issue tbd).

## v2026.04.30.3 — 2026-04-30

### fix: Database.Kind accepts ADR-0097 snake_case target types (#43)

`pkg/config/database.go` validator only allowed legacy CamelCase
(`OracleDatabase`, `PostgresDatabase`). DbSys manifests written per
ADR-0097 (Oracle Cloud Control target_type alignment) use the
snake_case form (`oracle_database`, `rac_database`, `pg_database`,
`pg_cluster`) and failed `proxctl kickstart generate` validation.

Now accepts both legacy + ADR-0097 forms. Sub-target types
(`oracle_pdb`, `oracle_listener`, `oracle_asm`, `oracle_instance`,
`oracle_home`, `oracle_dg_topology`, `cluster`, `host`) remain
forbidden as top-level kinds — they live under `spec.*`.

Caught during /lab-up Phase A.3 (infrastructure issue #479) on
2026-04-30.

## v2026.04.30.1 — 2026-04-30

### fix: kickstart pins disk partitioning to sda (#39)

`common/disk.tmpl` issued plain `clearpart --all` + `part /boot…` without
any `--ondisk` constraint. Anaconda autoselected the target disk by size
heuristic, so when the hypervisor manifest provisioned multiple disks
(scsi0=root 64G, scsi1=u01 100G, scsi2..4=asm 40+40+20G), the OS landed
on `sdb` (the largest by accident) instead of `sda` (storage_class=root).

Result: `linuxctl apply` failed at the disk manager — sdb was already
LVM-managed by the system, but linux.yaml's `additional` disk for u01
expected sdb to be free.

Fix: explicit `ignoredisk --only-use=sda`, `clearpart --drives=sda`,
`bootloader --boot-drive=sda`, `--ondisk=sda` on every `part`. Anaconda
now only touches sda; sdb…sde stay raw for post-install management
(linuxctl additional disk_layout, ASM disks via AFD).

## v2026.04.28.10 — 2026-04-29

### fix: nm_keyfile detects runtime interface name (#37)

v2026.04.28.9 wrote NM keyfiles using the manifest's NIC name (e.g.
`ens18`). On the installed OEL9 system the device showed up as
`enp6s18` — Anaconda's install env and the running system disagreed
about predictable interface naming. Result: keyfile bound to a name
that didn't exist; NM auto-created a DHCP profile for the real device.

Fix: `nm_keyfile.tmpl` now scans `/sys/class/net` at install time, picks
the first physical Ethernet device, and writes the keyfile keyed by both
that runtime device name AND its MAC. Auto-DHCP profiles created by NM
are removed first to prevent a race.

Live-caught: ext3adm1/ext4adm1 had keyfiles for `ens18`, but the kernel
exposed `enp6s18` to NM, so the static config was bound to a phantom
device.

## v2026.04.28.9 — 2026-04-29

### fix: NetworkManager keyfile for static NIC config on OEL8/OEL9 (#35)

Anaconda's `network --bootproto=static --activate --onboot=yes` writes
legacy `/etc/sysconfig/network-scripts/ifcfg-<dev>` files. On OEL9 (and
increasingly OEL8) the NetworkManager daemon prefers keyfile format
(`/etc/NetworkManager/system-connections/<id>.nmconnection`) and on first
boot auto-creates a fresh **DHCP** profile when no keyfile exists for the
interface. The legacy ifcfg file is silently ignored, so the installed
system comes up on DHCP — not the configured static IP.

Fix: new `common/nm_keyfile.tmpl` template emits a `%post` that writes a
NetworkManager keyfile per static NIC, removes any stale ifcfg/keyfile
that could create profile races, and sets `autoconnect-priority=100` so
NM doesn't create a competing default-named profile.

Wired into both `oraclelinux9/base.ks.tmpl` and `oraclelinux8/base.ks.tmpl`.
Live-verified: ext3adm1 / ext4adm1 came up at the wrong DHCP IPs (.103/.104)
on first install — the OPNsense static reservations were correct but
NM ignored them.

## v2026.04.28.8 — 2026-04-29

### fix: kickstart `reboot --eject` to break install loop (#33)

OEL8/OEL9 base.ks templates emitted plain `reboot`. After Anaconda finished
the install and rebooted, the install ISO stayed attached on `ide2`. With
SeaBIOS boot order `scsi0;ide3;ide2`, any time scsi0 was slow to handshake
or the freshly-installed bootloader hadn't been activated yet, BIOS fell
through to ide2 and re-launched Anaconda — install loop, indefinitely.

Fix: emit `reboot --eject` so Anaconda pops the install ISO on the way out;
the next boot has nothing on ide2 to fall through to.

Caught while running `/lab-up --phase B` for ext3+ext4 — VMs cycled through
the install screen multiple times before being diagnosed.

## v2026.04.28.7 — 2026-04-29

### fix: workflow up never attached kickstart ISO to VM (#31)

The single-VM workflow `apply` phase rendered/built/uploaded the kickstart
ISO to Proxmox storage but never attached it to the VM. The `create-vm`
step only attached the install ISO (ide2). With no OEMDRV-labeled CDROM
discoverable, Anaconda fell back to interactive mode and hung at the
language-select screen forever.

This affected BOTH the normal flow and `--skip-kickstart-build`. Symptom:
VMs run for hours with no install progress, no SSH, no ping; VM config
shows `boot=order=scsi0;ide2;net0` (default) and only `ide2`, no `ide3`.

`pkg/proxmox/boot.go` already shipped `AttachISOAsCDROM` and `SetBootOrder`
(plus a higher-level `ConfigureFirstBoot`) — they were just never wired
into the workflow.

Fix: new `attach-kickstart-iso` Change step inserted between `create-vm`
and `start-vm` in `Plan()`. `Apply()` calls
`AttachISOAsCDROM(node, vmid, "ide3", <ks-volid>)` followed by
`SetBootOrder(node, vmid, "scsi0;ide3;ide2")`. Step is conditional on
`iso.kickstart_storage` being configured (same gate as upload-iso /
verify-kickstart-iso).

Tests: `TestPlan` and `TestPlan_SkipKickstartBuild` updated for the new
6/4-step plans (was 5/3).

Caught while running `/lab-up --phase B` for ext3+ext4 — VMs ran 12+
hours with no progress until the workaround (manual ide3 attach + boot
reorder + restart) was applied.

## v2026.04.28.6 — 2026-04-28

### fix: CreateVM disk size suffix + EFIDisk format= rejection (#29)

Two more form-encoding bugs surfaced by live `/lab-up --phase B` of ext3 +
ext4 against proximo:

1. `DiskString` passed `<storage>:64G` literally to Proxmox. Proxmox's
   qemu-create API expects a bare integer (GiB). Anything with a unit
   suffix triggers `500 {"data":null}`. Fix: `normalizeSizeGiB` strips
   `G/GB/GiB`, scales `T/TB/TiB → ×1024`, and downscales `M/MB/MiB → /1024`
   when divisible.

2. `EFIDiskString` always emitted `format=raw`, which Proxmox rejects on
   `lvmthin`/`zfspool` storage. Same fix as DiskString in v2026.04.28.5:
   omit `format=` unless caller sets `EFIDiskSpec.Format`.

Tests in `pkg/proxmox/vm_test.go` updated. Live-verified: VM 2701
(ext3adm1) and 2702 (ext4adm1) both created and booted from the kickstart
ISO on proximo.

## v2026.04.28.5 — 2026-04-28

### fix: CreateVM form encoding for disk/NIC/tags (#27)

Three serializer bugs in `pkg/proxmox/vm.go` caused Proxmox to reject
`POST /nodes/{node}/qemu` with `400 Parameter verification failed`
(duplicate-key errors on `scsi0`, `net0`; tags split mid-string):

1. `DiskString` always emitted `format=raw`. Proxmox rejects an explicit
   `format=` on `lvmthin` / `zfspool` storage with
   "duplicate key in comma-separated list property: file". Fix: omit
   `format=` unless caller explicitly sets `DiskSpec.Format`.

2. `NICString` emitted bare `<model>,bridge=...` when MAC was empty/auto,
   which Proxmox parses as a duplicate `model` key. Fix: always emit
   `<model>=<mac>` (with `mac=auto` as the default sentinel).

3. `CreateVM` joined `Tags` with `;`. Proxmox's form parser splits the
   query body on `;` first, so multi-tag values were spilling into
   adjacent params. Fix: join with `,` (Proxmox's documented separator).

Tests in `pkg/proxmox/vm_test.go` updated for new expected wire forms.

Caught while running `/lab-up --phase B` for ext3+ext4 in
itunified-io/infrastructure (plan 034) — fifth gate after .1–.4.

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
