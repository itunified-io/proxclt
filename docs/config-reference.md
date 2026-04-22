# Configuration reference

Every YAML key proxctl recognises. Split-file layout: the `lab.yaml` master
manifest pulls the other layers in via `$ref`. You may also inline any layer
directly in `lab.yaml`.

- [lab.yaml (kind: Env)](#labyaml--kind-env)
- [hypervisor.yaml](#hypervisoryaml)
- [networks.yaml](#networksyaml)
- [storage-classes.yaml](#storage-classesyaml)
- [cluster.yaml](#clusteryaml)
- [linux.yaml](#linuxyaml)
- [Ref[T] resolution model](#reft-resolution-model)
- [Secret resolver](#secret-resolver)
- [Profile inheritance (`extends`)](#profile-inheritance-extends)
- [Validation](#validation)

The authoritative Go structs live under
[`pkg/config/`](https://github.com/itunified-io/proxctl/tree/main/pkg/config).
Field validation is driven by `go-playground/validator/v10` tags on those
structs — this page documents them.

---

## lab.yaml — kind: Env

Top-level master manifest. One per env. Filename is a convention, not
enforced — `lab.yaml`, `env.yaml`, and `prod.yaml` all work.

```yaml
version: "1"
kind: Env
metadata:
  name: rac-prod
  domain: example.com
  proxmox_context: prod-pve
  dbx_context: prod-dbx
  tags:
    owner: dba-team
    env: production
  description: "2-node RAC, prod cluster"
extends: oracle-rac-2node
spec:
  hypervisor:      { $ref: ./hypervisor.yaml }
  linux:           { $ref: ./linux.yaml }
  networks:        { $ref: ./networks.yaml }
  storage_classes: { $ref: ./storage-classes.yaml }
  cluster:         { $ref: ./cluster.yaml }
  databases:
    - $ref: ./db-orcl.yaml
hooks:
  pre_apply:
    - run: "echo entering apply for $PROXCTL_STACK"
  post_apply: []
  pre_destroy: []
  post_destroy: []
```

| Key | Type | Required | Notes |
|-----|------|----------|-------|
| `version`                       | string  | yes | Must be `"1"`. |
| `kind`                          | string  | yes | Must be `Env`. |
| `metadata.name`                 | string  | yes | RFC-1123 hostname-safe. Used in the env registry. |
| `metadata.domain`               | string  | no  | Default DNS suffix. |
| `metadata.proxmox_context`      | string  | no  | Default `--context` for this env. |
| `metadata.dbx_context`          | string  | no  | Handed through to `dbx` for database workflows. |
| `metadata.tags`                 | map     | no  | Copied to Proxmox VM `tags`. |
| `metadata.description`          | string  | no  | Free text. |
| `extends`                       | string  | no  | Profile name (shipped or under `~/.proxctl/profiles/`). |
| `spec.hypervisor`               | Ref     | yes | See [hypervisor.yaml](#hypervisoryaml). |
| `spec.linux`                    | Ref     | no  | Passthrough for linuxctl. |
| `spec.networks`                 | Ref     | yes | See [networks.yaml](#networksyaml). |
| `spec.storage_classes`          | Ref     | yes | See [storage-classes.yaml](#storage-classesyaml). |
| `spec.cluster`                  | Ref     | no  | See [cluster.yaml](#clusteryaml). Omit for host-only envs. |
| `spec.databases`                | []Ref   | no  | Database manifests handed through to `dbx`. |
| `hooks.pre_apply`               | []Hook  | no  | Run before `workflow up`. |
| `hooks.post_apply`              | []Hook  | no  | Run after a successful `workflow up`. |
| `hooks.pre_destroy`             | []Hook  | no  | Run before `workflow down`. |
| `hooks.post_destroy`            | []Hook  | no  | Run after `workflow down`. |

Each hook is `{ run: "cmd" }` — a shell command run with `PROXCTL_STACK`,
`PROXCTL_CONTEXT`, and `PROXCTL_HOME` in the environment.

---

## hypervisor.yaml

Describes the Proxmox-side topology — which physical node hosts which VM,
what VMID it gets, how much CPU/memory, which NICs, which disks, which
install ISO, and which kickstart template.

```yaml
kind: Hypervisor
defaults:
  resources:
    memory: 8192
    cores: 4
    sockets: 1
  tags: [proxctl]
nodes:
  rac-node-1:
    proxmox:
      node_name: pve-prod-1
      vm_id: 1001
    ips:
      public:  10.10.0.101
      private: 10.10.1.101
      vip:     10.10.0.201
    resources:
      memory: 16384
      cores: 8
      sockets: 1
      bios: ovmf
      machine: q35
    nics:
      - name: net0
        usage: public
        bridge: vmbr0
        network: public          # refers to networks.yaml zone
        ipv4:
          address: 10.10.0.101/24
          gateway: 10.10.0.1
          dns: [10.10.0.1]
      - name: net1
        usage: private
        bridge: vmbr1
        network: private
        ipv4:
          address: 10.10.1.101/24
    disks:
      - id: 0
        size: 64G
        storage_class: local-lvm # refers to storage-classes.yaml
        interface: scsi0
        role: root
      - id: 1
        size: 500G
        storage_class: shared-nfs
        interface: scsi1
        shared: true
        role: asm-data
        tag: asm-data-1
    tags: [rac, prod]
iso:
  storage: local
  image: OracleLinux-R9-U3-x86_64-dvd.iso
  guest_os_type: l26
  kickstart_storage: local
  bootloader_dir: /usr/share/xorriso/iso-bootloaders
kickstart:
  distro: oraclelinux9
  timezone: Europe/Berlin
  keyboard_layout: de
  lang: en_US.UTF-8
  mode: text
  ipv6: false
  chrony_servers: [0.pool.ntp.org, 1.pool.ntp.org]
  sudo:
    wheel_nopasswd: true
  packages:
    base: [chrony, open-vm-tools]
    post: [oracle-database-preinstall-23ai]
  firewall:
    enabled: false
  update_system: true
  ssh_keys:
    root: ["ssh-ed25519 AAAA... ops@example.com"]
  additional_users:
    - name: oracle
      wheel: false
      ssh_key: "ssh-ed25519 AAAA... oracle@example.com"
```

### Top-level

| Key         | Type                  | Required | Notes |
|-------------|-----------------------|----------|-------|
| `kind`      | string                | yes      | Must be `Hypervisor`. |
| `defaults`  | NodeDefaults          | no       | Merged under every `nodes[*]` entry. |
| `nodes`     | map<string, Node>     | yes      | ≥1 entry. Key is the logical node name. |
| `iso`       | ISOConfig             | no       | Install ISO storage + image. |
| `kickstart` | KickstartConfig       | no       | Kickstart inputs. |

### Node fields

| Key                 | Type              | Required | Notes |
|---------------------|-------------------|----------|-------|
| `proxmox.node_name` | string            | yes      | Physical Proxmox node. |
| `proxmox.vm_id`     | int               | yes      | 100–999999. Must be unique across the env. |
| `ips`               | map<string,IP>    | yes      | Role → IP mapping, e.g. `public: 10.10.0.101`. |
| `resources.memory`  | int (MiB)         | yes*     | ≥512. Inherited from `defaults` if unset. |
| `resources.cores`   | int               | yes*     | ≥1. |
| `resources.sockets` | int               | yes*     | ≥1. |
| `resources.cpu`     | string            | no       | Proxmox CPU type, e.g. `host`. |
| `resources.bios`    | `seabios`/`ovmf`  | no       | |
| `resources.machine` | string            | no       | e.g. `q35`. |
| `nics`              | []NIC             | no       | |
| `disks`             | []Disk            | no       | |
| `tags`              | []string          | no       | Applied to Proxmox VM. |

### NIC

| Key                   | Type     | Required | Notes |
|-----------------------|----------|----------|-------|
| `name`                | string   | yes      | `net0`, `net1`, … |
| `usage`               | string   | yes      | One of `public`, `vip`, `scan`, `private`, `management`. |
| `bridge`              | string   | no       | Proxmox bridge (e.g. `vmbr0`). |
| `mac`                 | string   | no       | Pin a MAC; otherwise auto. |
| `network`             | string   | no       | Zone name from `networks.yaml`. |
| `ipv4.address`        | CIDR     | no       | Static IPv4 (required if `bootproto: static`). |
| `ipv4.gateway`        | IP       | no       | |
| `ipv4.dns`            | []IP     | no       | |
| `ipv4_addresses`      | []IP     | no       | Secondary addresses (VIPs). |
| `bootproto`           | string   | no       | `static`, `dhcp`, `none`, `link`, `ibft`. |
| `controlled_by`       | string   | no       | `NetworkManager`, `crs`, or `networkd`. |
| `hostname_aliases`    | []string | no       | Emit into `/etc/hosts`. |
| `shared_with_cluster` | bool     | no       | Marks a RAC VIP NIC. |

### Disk

| Key             | Type    | Required | Notes |
|-----------------|---------|----------|-------|
| `id`            | int     | yes      | ≥0. |
| `size`          | string  | yes      | Proxmox sizing: `64G`, `500G`, `4T`. |
| `storage_class` | string  | no       | Role from `storage-classes.yaml`. |
| `storage`       | string  | no       | Override — use this Proxmox storage directly. |
| `interface`     | string  | no       | `scsi0` … `scsi9`. |
| `shared`        | bool    | no       | Shared disk (RAC ASM). Created once, attached many. |
| `tag`           | string  | no       | Correlates with `linux.yaml` disk-tag refs. |
| `role`          | string  | no       | Descriptive (`root`, `asm-data`, `redo`). |

### ISOConfig

| Key                | Type   | Required | Notes |
|--------------------|--------|----------|-------|
| `storage`          | string | yes      | Proxmox storage holding the install ISO. |
| `image`            | string | yes      | ISO filename on that storage. |
| `guest_os_type`    | string | no       | Proxmox `ostype` (e.g. `l26`). |
| `kickstart_storage`| string | no       | Storage to upload the first-boot ISO to. Defaults to `storage`. |
| `bootloader_dir`   | string | no       | Override xorriso bootloader directory. |

### KickstartConfig

| Key                | Type               | Required | Notes |
|--------------------|--------------------|----------|-------|
| `distro`           | string             | yes      | `oraclelinux8` / `oraclelinux9` / `ubuntu2204` / `rhel9` / `rocky9` / `sles15`. |
| `timezone`         | string             | no       | e.g. `Europe/Berlin`. |
| `keyboard_layout`  | string             | no       | |
| `lang`             | string             | no       | e.g. `en_US.UTF-8`. |
| `mode`             | `text`/`graphical` | no       | |
| `ipv6`             | bool               | no       | |
| `chrony_servers`   | []string           | no       | |
| `sudo.wheel_nopasswd` | bool            | no       | |
| `packages.base`    | []string           | no       | Installed in `%packages`. |
| `packages.post`    | []string           | no       | Installed in `%post`. |
| `firewall.enabled` | bool               | no       | |
| `update_system`    | bool               | no       | Runs `dnf update` / `apt upgrade` in `%post`. |
| `ssh_keys`         | map<user,[]string> | no       | |
| `additional_users` | []AdditionalUser   | no       | |

---

## networks.yaml

Catalogue of L3 zones referenced by `hypervisor.yaml:nodes[*].nics[*].network`.

```yaml
kind: Networks
public:
  cidr: 10.10.0.0/24
  gateway: 10.10.0.1
  dns: [10.10.0.1]
private:
  cidr: 10.10.1.0/24
vip:
  cidr: 10.10.0.0/24
```

| Key                   | Type     | Required | Notes |
|-----------------------|----------|----------|-------|
| `kind`                | string   | yes      | `Networks`. |
| `<zone>.cidr`         | CIDR     | yes      | |
| `<zone>.gateway`      | IP       | no       | |
| `<zone>.dns`          | []IP     | no       | |

At least one zone is required. Zone names are arbitrary but conventionally
`public`, `private`, `vip`, `management`, `scan`.

---

## storage-classes.yaml

Role → backend mapping. `hypervisor.yaml:disks[*].storage_class` refers here.

```yaml
kind: StorageClasses
local-lvm:
  backend: lvm-thin
  shared: false
shared-nfs:
  backend: nfs
  shared: true
asm-shared:
  backend: rbd
  shared: true
```

| Key                  | Type   | Required | Notes |
|----------------------|--------|----------|-------|
| `kind`               | string | yes      | `StorageClasses`. |
| `<class>.backend`    | string | yes      | Proxmox storage backend id (`lvm-thin`, `nfs`, `rbd`, `zfs`, `dir`, …). |
| `<class>.shared`     | bool   | no       | Whether the backing store is shared across nodes. Required true for RAC. |

---

## cluster.yaml

Cluster-level semantics. Only required for clustered workloads (RAC, PG HA).

```yaml
kind: Cluster
type: oracle-rac
scan_name: rac-scan.example.com
scan_ips:
  - 10.10.0.110
  - 10.10.0.111
  - 10.10.0.112
interconnect_subnet: 10.10.1.0/24
hosts_entries:
  - ip: 10.10.0.101
    names: [rac-node-1, rac-node-1.example.com]
  - ip: 10.10.0.201
    names: [rac-node-1-vip]
```

| Key                   | Type     | Required | Notes |
|-----------------------|----------|----------|-------|
| `kind`                | string   | yes      | `Cluster`. |
| `type`                | string   | no       | `oracle-rac`, `oracle-single`, `pg-single`, `pg-ha`, `plain`. |
| `scan_name`           | string   | no       | Oracle SCAN DNS name. |
| `scan_ips`            | []IP     | no       | SCAN IPs (typically 3 for RAC). |
| `interconnect_subnet` | CIDR     | no       | Dedicated interconnect network. |
| `hosts_entries`       | []Entry  | no       | Rendered into `/etc/hosts` on every node. |

---

## linux.yaml

Consumed by [`linuxctl`](https://github.com/itunified-io/linuxctl), not proxctl.
proxctl reads it only to cross-check disk tag references.

```yaml
kind: Linux
# ... arbitrary linuxctl body (partitions, OS tuning, users, etc.) ...
```

The only proxctl-enforced rule: every `disk.tag` referenced in `linux.yaml`
must exist in `hypervisor.yaml:disks[*].tag` within the same env.

---

## Ref[T] resolution model

`$ref` handling is implemented in
[`pkg/config/loader.go`](../pkg/config/loader.go).

- A YAML mapping node with a single scalar key `$ref` is treated as a file
  reference and loaded relative to the **parent file's directory**.
- Any other mapping is decoded inline as the corresponding type.
- If the referenced file is missing, validation fails with
  `ref <path>: open: no such file or directory`.
- Absolute and relative paths both work; relative is recommended (git-friendly).

```yaml
# Inline
networks:
  kind: Networks
  public: { cidr: 10.10.0.0/24 }

# Reference
networks: { $ref: ./networks.yaml }
```

Refs can be nested (a `linux.yaml` may itself `$ref` other files — proxctl
treats the linux layer opaquely, so nested refs there are linuxctl's problem).

---

## Secret resolver

Secrets in YAML **must** be supplied via placeholders. Plaintext values fail
validation for known sensitive fields.

Syntax: `${SOURCE:ARG}` with optional pipe filters: `${SOURCE:ARG | FILTER1 | FILTER2}`.

| Source | Meaning | Example |
|--------|---------|---------|
| `env`   | OS environment variable | `${env:PVE_TOKEN_SECRET}` |
| `file`  | File contents (trimmed) | `${file:/run/secrets/pve-token}` |
| `vault` | HashiCorp Vault KV v2   | `${vault:secret/data/pve#token}` |
| `gen`   | Generated (random, cached per env)     | `${gen:password,length=32}` |
| `ref`   | Reference another key in the same env   | `${ref:spec.hypervisor.nodes.rac-node-1.ips.public}` |

Filters (applied left to right):

| Filter    | Behaviour |
|-----------|-----------|
| `base64`  | base64-encode the resolved value |
| `default=X` | fall back to `X` if the source resolves empty |

```yaml
token_secret_ref: "${env:PVE_TOKEN_SECRET | default=dev-only-token}"
admin_password:   "${vault:secret/data/rac#admin | base64}"
```

Unresolved placeholders render as `***` in `proxctl config render` output,
never as their actual value. See the resolver implementation:
[`pkg/config/resolver.go`](../pkg/config/resolver.go).

---

## Profile inheritance (`extends`)

`extends: <name>` makes proxctl load the named profile (from the embedded
library, then `~/.proxctl/profiles/<name>.yaml` if present — user overrides
win) and **deep-merges** it under your env:

- Your env's keys always win.
- Maps are merged key-by-key.
- Lists are replaced wholesale (not concatenated) — redefine in your env to
  override.

See [profile-guide.md](profile-guide.md) for the shipped profiles and how to
write your own.

---

## Validation

Run `proxctl config validate <env.yaml>` after every edit. The validator
enforces both struct-level tags (`required`, `cidr`, `ip`, `oneof`, …) and
cross-file invariants:

- `nodes[*].proxmox.vm_id` unique within env
- `nodes[*].ips` values unique across all nodes
- Every `nics[*].network` references a zone defined in `networks.yaml`
- Every `disks[*].storage_class` references a class defined in
  `storage-classes.yaml`
- RAC envs (`cluster.type == oracle-rac`): ≥2 nodes, ≥1 `shared` disk,
  `interconnect_subnet` set
- Every `linux.yaml` disk tag reference exists in `hypervisor.yaml`

See the validator source for the authoritative list:
[`pkg/config/validators.go`](../pkg/config/validators.go).
