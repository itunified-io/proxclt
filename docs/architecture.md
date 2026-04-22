# Architecture

High-level view of how proxctl is organised. Targets readers who want to
contribute, audit, or embed proxctl in a larger system.

- [Component diagram](#component-diagram)
- [Data flow: env.yaml → Proxmox](#data-flow-envyaml--proxmox)
- [Package layout](#package-layout)
- [State model](#state-model)
- [Concurrency model](#concurrency-model)
- [Plugin points](#plugin-points)

---

## Component diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        proxctl CLI (Cobra)                       │
│                   cmd/proxctl + internal/root                    │
└──────────────────────────┬───────────────────────────────────────┘
                           │
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
   ┌───────────────┐┌──────────────┐┌──────────────┐
   │ pkg/license   ││ pkg/config   ││ pkg/workflow │
   │ JWT gate      ││ loader +     ││ plan / apply │
   │ ToolCatalog   ││ validators + ││ + rollback   │
   └───────────────┘│ resolver     │└──────┬───────┘
                    └──────┬───────┘       │
                           ▼               ▼
                    ┌──────────────┐┌──────────────┐
                    │ pkg/kickstart││ pkg/proxmox  │
                    │ template +   ││ REST client  │
                    │ xorriso ISO  ││ token auth   │
                    └──────┬───────┘└──────┬───────┘
                           │               │
                           ▼               ▼
                    ┌──────────────┐┌──────────────┐
                    │  xorriso     ││ Proxmox VE   │
                    │  (host tool) ││ REST API     │
                    └──────────────┘└──────────────┘

                    ┌──────────────────────────────┐
                    │ pkg/state (SQLite)           │
                    │ ~/.proxctl/state.db          │
                    │ audit log, inventory,        │
                    │ snapshot history             │
                    └──────────────────────────────┘
```

The CLI layer is thin — it parses flags, resolves the active context + env,
calls `license.Check()` against the `ToolCatalog`, and hands off to one of
the domain packages. All actual work (HTTP, ISO build, validation) lives in
`pkg/*`.

---

## Data flow: env.yaml → Proxmox

```
env.yaml (master)
  │
  │  pkg/config.Load()
  ▼
Ref[T] resolution  (loader walks $refs, inlines nested YAML)
  │
  ▼
Profile merge (extends: <name> → deep-merge under env)
  │
  ▼
Secret resolver (${env,file,vault,gen,ref:...} → real values)
  │
  ▼
Struct-level validation (go-playground/validator/v10)
  │
  ▼
Cross-file invariants (VMID uniqueness, NIC ↔ network, disk ↔ SC, RAC rules)
  │
  ▼
Resolved *config.Env handed to pkg/workflow
  │
  ├──► For each node:
  │       pkg/kickstart.Render() → rendered ks.cfg / user-data
  │       pkg/kickstart.BuildISO() → first-boot ISO on disk
  │       pkg/proxmox.UploadISO() → Proxmox storage
  │       pkg/proxmox.CreateVM()
  │       pkg/proxmox.AttachISO() + SetBootOrder()
  │       pkg/proxmox.StartVM()
  │       pkg/proxmox.WaitForAgent() / SSH probe
  │
  ▼
pkg/state.Record() — inventory + audit entry
```

Every stage has a corresponding Rollback step pushed onto a per-VM stack.
On any failure the stack is unwound in reverse, guaranteeing no orphaned
Proxmox resources.

---

## Package layout

| Package                   | Role |
|---------------------------|------|
| `cmd/proxctl`             | `main.go` — calls `internal/root.Execute()`. |
| `internal/root`           | Cobra command tree + flag wiring + `clientutil.go` (context → proxmox client). |
| `pkg/config`              | YAML loader, `$ref` resolver, profile merger, secret resolver, validators, JSON Schema export. |
| `pkg/config/profiles`     | Embedded shipped profiles (`go:embed`). |
| `pkg/kickstart`           | `DistroProfile` interface, `text/template` renderer, xorriso ISO builder, embedded templates. |
| `pkg/proxmox`             | REST client with token auth, task poll, VM CRUD, storage, ISO, snapshot, boot-order ops. |
| `pkg/workflow`            | `SingleVMWorkflow` + `MultiNodeWorkflow` orchestrators with rollback. |
| `pkg/license`             | JWT gate + `ToolCatalog`. |
| `pkg/state`               | SQLite (`modernc.org/sqlite` — pure Go) inventory + audit log. |
| `pkg/version`             | ldflags-injected build info. |
| `internal/testutil`       | Test helpers (httptest Proxmox fake, tempdir fixtures). |

Dependency rule: `pkg/*` imports only from `pkg/*`; `internal/*` is free to
import from `pkg/*` but not vice versa. This keeps the public API clean and
lets external callers embed `pkg/config` + `pkg/workflow` without pulling
in CLI wiring.

---

## State model

SQLite at `~/.proxctl/state.db`. Schema (abridged):

```sql
CREATE TABLE vms (
    env         TEXT NOT NULL,
    node_name   TEXT NOT NULL,   -- logical node from hypervisor.yaml
    proxmox_node TEXT NOT NULL,
    vm_id       INTEGER NOT NULL,
    state       TEXT NOT NULL,   -- planned, creating, running, stopped, destroyed
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL,
    PRIMARY KEY (env, node_name)
);

CREATE TABLE snapshots (
    env        TEXT NOT NULL,
    node_name  TEXT NOT NULL,
    name       TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (env, node_name, name)
);

CREATE TABLE audit (
    id         INTEGER PRIMARY KEY,
    ts         TIMESTAMP NOT NULL,
    operator   TEXT NOT NULL,
    tool       TEXT NOT NULL,   -- e.g. workflow.up
    env        TEXT,
    context    TEXT,
    result     TEXT NOT NULL,   -- ok / err
    duration_ms INTEGER,
    message    TEXT,
    hash       BLOB             -- Enterprise: SHA-256 hash-chain
);
```

State is **advisory** — Proxmox itself is the source of truth. `proxctl vm
list --reconcile` refreshes the `vms` table from Proxmox and prunes stale
entries.

The Enterprise tier enables:

- `audit.hash` populated with `SHA256(prev_hash || row)` for tamper-evidence.
- Optional central sync to S3/R2/GCS via a background writer.

---

## Concurrency model

Workflow execution:

- **SingleVMWorkflow** — strictly serial. Used when `len(env.nodes) == 1`.
  Trivial to reason about; no synchronisation needed.
- **MultiNodeWorkflow** — one goroutine per node, wrapped in
  `golang.org/x/sync/errgroup`. Default mode is fail-fast: first error
  cancels the `errgroup.WithContext` and other goroutines see the cancel
  signal.
- `ContinueOnError` mode uses a plain `sync.WaitGroup` + errors channel,
  aggregates all failures, and returns an `errors.Join`-style combined
  error.
- **Shared ISO upload mutex** — `*sync.Mutex` passed into every
  `SingleVMWorkflow.UploadMu` so that multiple nodes uploading to the same
  storage serialise, preventing duplicate uploads and Proxmox `409`
  conflicts.

`go test -race ./...` covers the multi-node paths with an atomic counter
that asserts ≤1 in-flight upload at any time
(`TestMultiNode_Apply_ISOUploadSerialized`).

Proxmox-side concurrency is governed by the task API; proxctl polls each
task's UPID with exponential backoff (100 ms → 5 s, cap 30 s per request).

---

## Plugin points

proxctl keeps the ingest pipeline pluggable in three places:

### 1. DistroProfile

New Linux distros plug in via the `DistroProfile` interface in `pkg/kickstart`.
See [distro-guide.md](distro-guide.md#distroprofile-interface).

### 2. Secret resolvers

`pkg/config/resolver.go` exposes a `Resolver` interface and a registration
function. Built-ins: `env`, `file`, `vault`, `gen`, `ref`. Downstream
embeddings (e.g. a CI-specific secret backend) register a new source and it
becomes available in placeholder syntax.

### 3. Workflow hooks

`lab.yaml:hooks.{pre,post}_{apply,destroy}` run arbitrary shell commands at
defined workflow phases. Use these for custom notifications, DNS updates,
or CMDB sync — proxctl does not ship those integrations directly because
they are site-specific.

---

Further reading:

- Public design narrative: the CHANGELOG + PR history on
  [github.com/itunified-io/proxctl](https://github.com/itunified-io/proxctl).
- Private design doc: `docs/plans/024-proxctl-design.md` in
  `itunified-io/infrastructure` (internal).
