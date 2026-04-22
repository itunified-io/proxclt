# Profile guide

Profiles are reusable env baselines. An env sets `extends: <profile-name>`
and inherits every field the profile defines, then overrides only what it
needs. Profile support is **free tier** (Community); profile CRUD commands
(writing new profiles via the CLI) are Business tier.

- [What profiles are](#what-profiles-are)
- [Shipped profiles](#shipped-profiles)
  - [oracle-rac-2node](#oracle-rac-2node)
  - [pg-single](#pg-single)
  - [host-only](#host-only)
- [Writing a custom profile](#writing-a-custom-profile)
- [Inheritance rules](#inheritance-rules)

---

## What profiles are

A profile is a regular `kind: Env` YAML document that lives either:

- **Embedded** inside the proxctl binary at
  [`pkg/config/profiles/*.yaml`](../pkg/config/profiles), or
- **User-provided** at `~/.proxctl/profiles/<name>.yaml`

Load order is user-overrides-shipped: if both exist, the user file wins.

Profiles set **defaults only**. You still need a concrete env with real
`vm_id`, `ips`, `bridge`, and `storage_class` values before you can apply
anything.

List what is shipped:

```bash
proxctl workflow profile list
proxctl workflow profile show oracle-rac-2node
```

---

## Shipped profiles

### oracle-rac-2node

Baseline for a 2-node Oracle Real Application Clusters deployment.

**Use case.** Production or lab RAC on Proxmox with shared storage for ASM
disks (NFS, RBD, or iSCSI LUN). Interconnect on a dedicated private bridge.

**Node template.** Two logical nodes (`rac-node-1`, `rac-node-2`); each
inherits:

- 8 cores / 16 GiB memory (override via `resources:`)
- `bios: ovmf`, `machine: q35`
- NICs: `net0` (public), `net1` (private interconnect), with VIP + SCAN
  usage markers
- Disks: `scsi0` root (64 GiB) + at least one `shared: true` ASM disk
- `cluster.type: oracle-rac`
- `kickstart.distro: oraclelinux9` + `oracle-database-preinstall-23ai`
  package in `packages.post`

**Required env overrides.** You must supply:

- `nodes.rac-node-1.proxmox.vm_id` + `nodes.rac-node-2.proxmox.vm_id`
- `nodes.rac-node-1.proxmox.node_name` + `nodes.rac-node-2.proxmox.node_name`
- `nodes.*.ips.{public,private}` (real IPs on your public/private zones)
- `networks.*` zones (public + private CIDRs that match your Proxmox bridges)
- `storage_classes.*` (`asm-shared` pointing at a shared backend)
- `cluster.scan_name` + `cluster.scan_ips` (3 SCAN IPs typical)
- `cluster.interconnect_subnet`
- `iso.image` (OL9 DVD ISO filename)

**Example env.yaml.**

```yaml
version: "1"
kind: Env
metadata:
  name: rac-prod
  domain: example.com
  proxmox_context: prod-pve
extends: oracle-rac-2node
spec:
  hypervisor: { $ref: ./hypervisor.yaml }
  networks:   { $ref: ./networks.yaml }
  storage_classes: { $ref: ./storage-classes.yaml }
  cluster:    { $ref: ./cluster.yaml }
```

Full example at [`docs/examples/oracle-rac-2node/`](examples/oracle-rac-2node).

### pg-single

Baseline for a single-node PostgreSQL deployment. Good for dev, staging, or
small production where HA is not required.

**Use case.** One VM running PostgreSQL with a dedicated data disk.

**Node template.**

- 1 logical node (`pg-1`)
- 4 cores / 8 GiB memory
- `scsi0` root (64 GiB) + `scsi1` data (configurable) on local or shared storage
- `cluster.type: pg-single`
- `kickstart.distro: ubuntu2204` or `oraclelinux9`

**Required env overrides.**

- `nodes.pg-1.proxmox.{node_name,vm_id}`
- `nodes.pg-1.ips.public`
- `networks.public` + `storage_classes` entries

**Example env.yaml.**

```yaml
version: "1"
kind: Env
metadata:
  name: pg-stg
extends: pg-single
spec:
  hypervisor: { $ref: ./hypervisor.yaml }
  networks:   { $ref: ./networks.yaml }
  storage_classes: { $ref: ./storage-classes.yaml }
```

Full example at [`docs/examples/pg-single/`](examples/pg-single).

### host-only

Minimal Linux host baseline — one VM, no cluster, no database. Used by the
host-monitoring test suite and as the default for `proxctl stack new`.

**Use case.** Smoke-test your Proxmox + proxctl setup, or provision a plain
Linux box (jumpbox, monitoring, utility VM).

**Node template.**

- 1 logical node (`host-1`)
- 2 cores / 4 GiB memory
- `scsi0` root (32 GiB) on local storage
- No cluster section
- `kickstart.distro: ubuntu2204`

**Required env overrides.**

- `nodes.host-1.proxmox.{node_name,vm_id}`
- `nodes.host-1.ips.public`

**Example env.yaml.**

```yaml
version: "1"
kind: Env
metadata:
  name: jumpbox
extends: host-only
spec:
  hypervisor: { $ref: ./hypervisor.yaml }
  networks:   { $ref: ./networks.yaml }
  storage_classes: { $ref: ./storage-classes.yaml }
```

Full example at [`docs/examples/host-only/`](examples/host-only).

---

## Writing a custom profile

Drop a YAML file under `~/.proxctl/profiles/`:

```bash
mkdir -p ~/.proxctl/profiles
$EDITOR ~/.proxctl/profiles/my-rac-4node.yaml
```

The file is a standard `kind: Env` manifest. Populate the fields you want to
bake in as defaults; leave the rest blank for concrete envs to fill.

```yaml
# ~/.proxctl/profiles/my-rac-4node.yaml
version: "1"
kind: Env
metadata:
  name: my-rac-4node-profile
  description: Internal 4-node RAC baseline
spec:
  hypervisor:
    kind: Hypervisor
    defaults:
      resources:
        memory: 32768
        cores: 16
        sockets: 2
        bios: ovmf
        machine: q35
      tags: [rac, prod]
    nodes: {}   # operator fills in rac-node-1..4
    kickstart:
      distro: oraclelinux9
      timezone: Europe/Berlin
      update_system: true
  networks:
    kind: Networks
  storage_classes:
    kind: StorageClasses
  cluster:
    kind: Cluster
    type: oracle-rac
```

Reference it from an env:

```yaml
extends: my-rac-4node
```

`proxctl workflow profile list` now shows `my-rac-4node` alongside the
shipped ones.

If you name your user profile the same as a shipped profile (e.g.
`oracle-rac-2node.yaml`), **the user profile wins**. This is the recommended
way to pin organisation-wide defaults.

---

## Inheritance rules

Merge order, shallow to deep:

1. Shipped profile (if `extends` matches a built-in).
2. User profile at `~/.proxctl/profiles/<extends>.yaml` (if present) —
   overrides the shipped one.
3. The concrete env's `spec.*` block — overrides both.

Merge semantics:

| Type            | Behaviour |
|-----------------|-----------|
| Scalars         | Concrete env wins over profile. |
| Maps (e.g. `nodes`, `zones`, `storage_classes.*`) | Deep-merged key by key. Env-only keys added; profile-only keys inherited; colliding keys use env value. |
| Lists (e.g. `nics`, `disks`, `scan_ips`, `chrony_servers`) | **Replaced wholesale** by the env if the env sets the key. To extend a profile list, repeat the profile entries in your env. |

Implications:

- You **cannot** inherit `nics: [net0 public]` from a profile and add
  `net1 private` in your env — redefine the full list.
- You **can** inherit `resources:` defaults from a profile and override just
  `memory` in your env (because `resources` is a map).
- Missing required fields after merge fail validation — run
  `proxctl config validate` to confirm.
