# proxctl user guide

This is the long-form walkthrough for day-to-day use. Start with
[quick-start.md](quick-start.md) if you have not run `proxctl` before.

Table of contents:

- [Concepts](#concepts)
- [Managing Proxmox contexts](#managing-proxmox-contexts)
- [The env registry](#the-env-registry)
- [Profile library](#profile-library)
- [The split-file env layout](#the-split-file-env-layout)
- [VM lifecycle](#vm-lifecycle)
- [Kickstart generation and ISO upload](#kickstart-generation-and-iso-upload)
- [Workflow orchestration](#workflow-orchestration)
- [Single-node vs multi-node workflows](#single-node-vs-multi-node-workflows)
- [Profile inspection (`profile list/show`)](#profile-inspection)
- [Rollback](#rollback)
- [Troubleshooting and the audit log](#troubleshooting-and-the-audit-log)

---

## Concepts

proxctl borrows the kubectl mental model and extends it with a small number of
terms:

| Term      | Meaning |
|-----------|---------|
| **context** | A named Proxmox endpoint + API token. Stored in `~/.proxctl/config.yaml`. Selected via `--context` or `proxctl config use-context`. |
| **env**     | A concrete VM environment (one or more VMs + network/storage semantics). Defined by an `env.yaml` master manifest plus referenced layer files. |
| **profile** | A reusable baseline shipped inside proxctl (`oracle-rac-2node`, `pg-single`, `host-only`) or provided by the operator under `~/.proxctl/profiles/`. Envs `extends:` a profile to inherit defaults. |
| **layer**   | One of the five split-file manifests (`hypervisor`, `linux`, `networks`, `storage-classes`, `cluster`). Each is a reusable YAML document referenced from the master `env.yaml` via `$ref`. |

**proxctl owns** the hypervisor, networks, storage-classes, cluster, and kickstart
layers — these fully describe the Proxmox-side provisioning. The `linux.yaml`
layer is passed through unchanged; proxctl reads it only for cross-file
validation (disk-tag references). `linuxctl` consumes it as the source of truth
for in-guest configuration.

---

## Managing Proxmox contexts

A **context** is what you log into. Mirrors `kubectl config`:

```bash
# Add / update
proxctl config use-context prod-pve \
  --endpoint https://pve01.prod.example.com:8006/api2/json \
  --token-id 'proxctl@pve!cicd' \
  --token-secret "$PVE_TOKEN_SECRET"

# Switch
proxctl config use-context lab-pve

# Inspect
proxctl config current-context
proxctl config get-contexts

# One-shot override for a single command
proxctl --context prod-pve vm list
```

Context config lives in `~/.proxctl/config.yaml`. The token secret is
**never** written to disk in plaintext — only a reference like
`${env:PVE_TOKEN_SECRET}`, `${file:/run/secrets/pve}`, or
`${vault:secret/data/pve#token}` (see
[secret resolver](config-reference.md#secret-resolver)).

If you manage several clusters (lab + prod + DR), keep a context for each.
Every subcommand accepts `--context NAME` to override the active one.

---

## The env registry

Envs are stored wherever you want them on disk (typically a git repo). The
registry `~/.proxctl/stacks.yaml` is just a **bookmark file** that maps short
names to paths — identical to `~/.kube/config` for envs.

```bash
# Scaffold + register a new env
proxctl stack new rac-prod --from oracle-rac-2node --dir ./envs/rac-prod

# Register an existing env
proxctl stack add rac-prod ./envs/rac-prod/lab.yaml

# List / switch / show
proxctl stack list
proxctl stack use rac-prod
proxctl stack current
proxctl stack show rac-prod

# Remove bookmark (does not delete files)
proxctl stack remove rac-prod
```

Every subcommand that needs an env accepts `--stack NAME_OR_PATH`:

```bash
proxctl workflow plan --stack rac-prod
proxctl workflow plan --stack ./envs/rac-prod/lab.yaml
```

---

## Profile library

Profiles are shipped baselines. See [profile-guide.md](profile-guide.md) for
the details.

```bash
proxctl workflow profile list
# → NAME                USE CASE
#    oracle-rac-2node    2-node Oracle RAC on shared storage
#    pg-single           Single PostgreSQL node
#    host-only           Minimal Linux host for host-monitoring

proxctl workflow profile show oracle-rac-2node
# prints the embedded YAML so you can see what fields you will inherit
```

Extend a profile from your env's `lab.yaml`:

```yaml
version: "1"
kind: Env
metadata:
  name: rac-prod
extends: oracle-rac-2node
spec:
  hypervisor:
    $ref: ./hypervisor.yaml
  # … only the fields you need to override
```

Deep-merge rules: your env's fields always win over the profile; unset fields
inherit from the profile.

User profiles under `~/.proxctl/profiles/<name>.yaml` are merged in **before**
built-in profiles of the same name, letting you pin organisation-wide
defaults.

---

## The split-file env layout

proxctl's master manifest is intentionally thin — it only references the
layer files:

```yaml
# lab.yaml
version: "1"
kind: Env
metadata:
  name: rac-prod
  domain: example.com
  proxmox_context: prod-pve
extends: oracle-rac-2node
spec:
  hypervisor:     { $ref: ./hypervisor.yaml }
  networks:       { $ref: ./networks.yaml }
  storage_classes:{ $ref: ./storage-classes.yaml }
  cluster:        { $ref: ./cluster.yaml }
  linux:          { $ref: ./linux.yaml }       # opaque to proxctl
```

- `hypervisor.yaml` — nodes, VMIDs, resources, NICs, disks, ISO storage, kickstart
- `networks.yaml` — IPv4 zone catalogue (public, private, vip, …)
- `storage-classes.yaml` — role → backend mapping (`local-lvm`, `shared-nfs`, …)
- `cluster.yaml` — cluster-level semantics (RAC SCAN IPs, interconnect subnet, `/etc/hosts` entries)
- `linux.yaml` — consumed by `linuxctl`, not proxctl

Every key is documented in [config-reference.md](config-reference.md).

You can always inline the entire env in a single `lab.yaml` by replacing each
`$ref` with the full layer body. The split-file model is the recommended
layout because it lets multiple envs share the same `networks.yaml` or
`storage-classes.yaml`.

---

## VM lifecycle

Per-VM commands (`proxctl vm …`) operate on one VM at a time. Use them for
ad-hoc interventions; use `proxctl workflow …` for env-level orchestration.

```bash
# Create a VM from the env's hypervisor.yaml (does not boot)
proxctl vm create rac-node-1 --stack rac-prod

# Power ops
proxctl vm start  rac-node-1 --stack rac-prod
proxctl vm stop   rac-node-1 --stack rac-prod --graceful
proxctl vm reboot rac-node-1 --stack rac-prod

# Inventory
proxctl vm list --stack rac-prod
proxctl vm status rac-node-1 --stack rac-prod

# Destroy
proxctl vm delete rac-node-1 --stack rac-prod --yes
```

Snapshots:

```bash
proxctl snapshot create rac-node-1 pre-patch --stack rac-prod
proxctl snapshot list   rac-node-1 --stack rac-prod
proxctl snapshot restore rac-node-1 pre-patch --stack rac-prod
proxctl snapshot delete rac-node-1 pre-patch --stack rac-prod
```

All lifecycle commands are **idempotent** where the Proxmox API allows it —
re-running `vm create` on an existing VMID logs a no-op instead of erroring.

---

## Kickstart generation and ISO upload

proxctl renders unattended-install configs from the env's
`hypervisor.yaml:kickstart` section and bundles them into a first-boot ISO
via `xorriso` (falls back to `mkisofs`).

```bash
# Render the kickstart to stdout (no side effects)
proxctl kickstart generate rac-node-1 --stack rac-prod

# Build + upload the first-boot ISO
proxctl kickstart build-iso rac-node-1 --stack rac-prod --out /tmp/ks.iso
proxctl kickstart upload /tmp/ks.iso --stack rac-prod --storage local

# List supported distros
proxctl kickstart distros
```

At install time proxctl also knows how to configure the boot order so the
first boot uses the install ISO and subsequent boots fall through to disk:

```bash
proxctl boot configure-first-boot rac-node-1 --stack rac-prod
proxctl boot eject-iso            rac-node-1 --stack rac-prod
```

Supported distros at launch: Oracle Linux 8, Oracle Linux 9, Ubuntu 22.04.
RHEL 9 / Rocky 9 / SLES 15 are drop-in — see
[distro-guide.md](distro-guide.md).

---

## Workflow orchestration

Workflows are the preferred entry point. A workflow takes one env and executes
the full `plan → apply → verify` pipeline across every VM.

```bash
# Dry-run
proxctl workflow plan --stack rac-prod

# Apply
proxctl workflow up --stack rac-prod --yes

# Inspect
proxctl workflow status --stack rac-prod
proxctl workflow verify --stack rac-prod

# Teardown
proxctl workflow down --stack rac-prod --yes
```

`up` internally calls, per node:

1. Render + build first-boot ISO.
2. Upload ISO to hypervisor storage (serialized across nodes; see below).
3. Create VM on Proxmox.
4. Attach ISO + set boot order.
5. Power on.
6. Poll for SSH reachability (verification step).

`down` stops and deletes every VM declared in the env. Shared disks (`shared:
true` in `hypervisor.yaml`) are only removed once.

Every workflow step is streamed to the audit log
(`~/.proxctl/audit.log`) with the tool name, operator identity, and timestamp.

---

## Single-node vs multi-node workflows

If the env has **one** node, proxctl runs `SingleVMWorkflow` — strictly serial,
simple error messages.

If the env has **two or more** nodes, proxctl automatically switches to
`MultiNodeWorkflow`:

- Each node runs its own workflow concurrently via `errgroup`.
- ISO uploads to the same Proxmox storage are serialized through a shared
  mutex so you never double-upload the same image.
- Default mode is **fail-fast** — the first node error cancels the rest.
- Pass `--continue-on-error` to keep going and aggregate errors at the end
  (Business tier).
- `--parallel N` caps concurrency (Business tier). With the Community license
  concurrency is forced to 1 even for multi-node envs.

```bash
proxctl workflow up --stack rac-prod --continue-on-error --parallel 2
```

---

## Profile inspection

```bash
proxctl workflow profile list
proxctl workflow profile show host-only
```

`profile show` prints the exact YAML that will be deep-merged under your env.
Use this to understand what defaults you are inheriting before you override.

---

## Rollback

Every workflow keeps an in-memory rollback stack per VM. On any `Apply` failure
the stack is unwound automatically: ISO detached, VM stopped, VM destroyed (in
that order). No orphaned resources should survive a failed `up`.

Manual rollback is also available:

```bash
# Restore from a snapshot
proxctl snapshot restore rac-node-1 pre-patch --stack rac-prod

# Full teardown + re-apply
proxctl workflow down --stack rac-prod --yes
proxctl workflow up   --stack rac-prod --yes
```

Snapshots are the recommended pre-patch checkpoint because they are cheap on
thin-provisioned storage and roll back in seconds.

---

## Troubleshooting and the audit log

When a command fails, add `--verbose` to see:

- Every HTTP call to Proxmox (method, path, status, task UPID)
- Kickstart template input + rendered output
- Secret resolver traces (with values **redacted**)
- Workflow state transitions

The audit log at `~/.proxctl/audit.log` captures every tool invocation:

```
2026-04-22T10:15:03Z op=buecheleb tool=workflow.up env=rac-prod context=prod-pve result=ok duration=47s
2026-04-22T10:16:01Z op=buecheleb tool=vm.delete node=rac-node-1 result=err msg="vm is running"
```

Enterprise tier adds a hash-chain over the log (tamper-evident) and optional
central sync to an object store — see [licensing.md](licensing.md).

Common failure scenarios are catalogued in
[troubleshooting.md](troubleshooting.md).
