# Quick start

Stand up your first VM on Proxmox in under 5 minutes. This walkthrough assumes
a reachable Proxmox 8.x cluster and an API token.

Prerequisites:

- `proxctl` installed (see [installation.md](installation.md))
- A Proxmox API token (`root@pam!proxctl` or a dedicated service user)
- One Proxmox node with an ISO storage (e.g. `local`) and a VM storage
  (e.g. `local-lvm`)
- One free VMID

---

## 1. Install

Pick a method from [installation.md](installation.md). The shortest path on a
dev box:

```bash
go install github.com/itunified-io/proxctl/cmd/proxctl@latest
proxctl version
```

## 2. Register your Proxmox context

```bash
export PVE_TOKEN_SECRET='your-token-uuid-here'

proxctl config use-context lab-pve \
  --endpoint https://pve.lab.example.com:8006/api2/json \
  --token-id 'root@pam!proxctl' \
  --token-secret "$PVE_TOKEN_SECRET"

proxctl config current-context      # → lab-pve
proxctl config get-contexts         # → NAME     ENDPOINT
                                    #    lab-pve  https://...
```

The context is persisted to `~/.proxctl/config.yaml`. The token secret is
stored **by reference** (`${env:PVE_TOKEN_SECRET}`) and resolved at runtime —
never written to disk in plaintext.

## 3. Scaffold your first env

The simplest profile is `host-only` — a single Linux VM with no cluster
semantics. Great for validating the install end-to-end.

```bash
proxctl stack new my-first-vm --from host-only --dir ./envs/my-first-vm
```

This lays down a split-file env:

```
envs/my-first-vm/
├── lab.yaml             # master manifest (kind: Env, extends: host-only)
├── hypervisor.yaml      # node -> Proxmox + VMID + IPs + NICs + disks
├── networks.yaml        # network zones
├── storage-classes.yaml # role -> backend mapping
└── linux.yaml           # passthrough (linuxctl reads this)
```

Edit `hypervisor.yaml` and set a real `node_name`, `vm_id`, and `ipv4.address`.

## 4. Validate + render

```bash
proxctl config validate ./envs/my-first-vm/lab.yaml
# → OK: my-first-vm

proxctl config render ./envs/my-first-vm/lab.yaml
# → YAML with all $refs resolved, secrets redacted as "***"
```

`render` is the safe way to inspect what proxctl will actually hand to the
Proxmox API — always check this before running `up`.

## 5. Plan

```bash
proxctl workflow plan --stack ./envs/my-first-vm/lab.yaml
```

Output shows the ordered actions proxctl will take — VM create, ISO upload
(if needed), first-boot ISO attach, boot-order set, power-on. No changes
applied yet.

## 6. Apply

```bash
proxctl workflow up --stack ./envs/my-first-vm/lab.yaml --yes
```

Progress is streamed on stderr; the final state summary goes to stdout
(JSON with `--json`). Under the hood:

1. Render kickstart file from the embedded distro template.
2. Build a first-boot ISO containing the kickstart (xorriso).
3. Upload the ISO to the configured Proxmox storage (idempotent).
4. Create the VM with the requested CPU/memory/disks/NICs.
5. Attach the first-boot ISO and set boot order.
6. Start the VM — it boots into unattended install.

## 7. Verify

```bash
proxctl workflow status --stack ./envs/my-first-vm/lab.yaml
# → NODE        VMID  STATE    IP              SSH
#    host-a     101   running  10.10.0.50      reachable

proxctl workflow verify --stack ./envs/my-first-vm/lab.yaml
# → all reachability checks passed
```

If `verify` times out waiting for SSH, check the VM console from the Proxmox
UI — kickstart errors surface there. See
[troubleshooting.md](troubleshooting.md#kickstart-boot-failures).

## 8. Teardown

```bash
proxctl workflow down --stack ./envs/my-first-vm/lab.yaml --yes
# stops + destroys the VM; preserves the env manifest
```

To also delete the env registry entry:

```bash
proxctl stack remove my-first-vm
```

---

## Next steps

- Full walkthrough: [user-guide.md](user-guide.md)
- All config keys: [config-reference.md](config-reference.md)
- Write your own profile: [profile-guide.md](profile-guide.md)
- CLI reference: [cli-reference.md](cli-reference.md)
