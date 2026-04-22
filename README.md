# proxctl

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8.svg)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/itunified-io/proxctl)](https://goreportcard.com/report/github.com/itunified-io/proxctl)
[![Release](https://img.shields.io/github/v/release/itunified-io/proxctl?display_name=tag&sort=semver)](https://github.com/itunified-io/proxctl/releases)

**proxctl** is a single-binary Go CLI for Proxmox VM provisioning — kickstart
rendering, ISO remastering, VM lifecycle, snapshots, and multi-VM workflow
orchestration, driven by a concise split-file YAML manifest with profile
inheritance and a signed-license tier model.

---

## 30-second demo

```bash
# 1. Install + register your Proxmox context
go install github.com/itunified-io/proxctl/cmd/proxctl@latest
proxctl config use-context lab-pve \
  --endpoint https://pve.lab.example.com:8006/api2/json \
  --token-id 'root@pam!proxctl' \
  --token-secret "$PVE_TOKEN_SECRET"

# 2. Scaffold + validate a stack
proxctl stack new my-first-vm --from host-only --dir ./envs/my-first-vm
proxctl config validate ./envs/my-first-vm/lab.yaml

# 3. Apply
proxctl workflow up --stack ./envs/my-first-vm/lab.yaml --yes
```

That's it — one command provisions the VM, uploads the first-boot ISO, boots
it into unattended install, and verifies SSH reachability.

---

## Links

- [Quick start](docs/quick-start.md) — 5-minute walkthrough
- [Installation](docs/installation.md) — Homebrew, binary, Docker, source, air-gap
- [User guide](docs/user-guide.md) — concepts, workflows, troubleshooting
- [Config reference](docs/config-reference.md) — every YAML key
- [CLI reference](docs/cli-reference.md) — auto-generated Cobra docs
- [Example envs](docs/examples/) — three validated profiles
- [Licensing](docs/licensing.md) — tier model + pricing

---

## Key features

- **Split-file manifest** — `hypervisor`, `networks`, `storage-classes`,
  `cluster`, `linux` layers referenced from a thin `lab.yaml`. Deep-merge
  profile inheritance via `extends: <name>`.
- **Three shipped profiles** — `oracle-rac-2node`, `pg-single`, `host-only`
  cover the 80% case; custom profiles drop into `~/.proxctl/profiles/`.
- **Unattended install** — kickstart / autoinstall / AutoYaST rendered from
  embedded templates for OL8, OL9, Ubuntu 22.04 (RHEL 9, Rocky 9, SLES 15
  drop-in). First-boot ISO built with xorriso.
- **Full VM lifecycle** — create, start, stop, reboot, delete, list, status;
  snapshots; boot-order management; first-boot ISO ejection.
- **Workflow orchestration** — `plan`, `up`, `down`, `status`, `verify`.
  Auto-dispatches to multi-node mode with concurrent per-node execution,
  shared-ISO-upload mutex, fail-fast or aggregate-errors modes, rollback.
- **kubectl-style contexts** — multiple Proxmox clusters side-by-side; switch
  via `proxctl config use-context`.
- **Secret resolver** — `${env,file,vault,gen,ref:...}` with `| base64` and
  `| default=` filters. Secrets never written to disk.
- **SQLite state + audit log** at `~/.proxctl/`. Enterprise tier adds a
  hash-chain and central sync.
- **License gate** — Community (AGPL) / Business (€99/seat/mo) / Enterprise
  (custom). Core workflow is free forever.

---

## Tiers

| Tier       | Price        | Includes |
|------------|--------------|----------|
| Community  | Free (AGPL)  | Full VM provisioning + workflow (serial). |
| Business   | €99/mo/seat  | Profile CRUD, parallel workflows, drift detection, REST API. |
| Enterprise | from €25k/yr | Audit hash-chain, central state sync, RBAC, air-gapped bundles, SLA. |

Bundle discounts available with `linuxctl` and `dbx` — see
[docs/licensing.md](docs/licensing.md).

---

## Status

**Phase 6: stable.** Core packages (`pkg/config`, `pkg/proxmox`,
`pkg/kickstart`, `pkg/workflow`, `pkg/license`, `internal/root`) all hold
**≥95% coverage**. `go test -race ./...` is green including the multi-node
workflow's shared-ISO-upload mutex. Community tier is production-ready; the
Business/Enterprise licence gate binds in Phase 7 when the dbx license
service wires up.

See the [CHANGELOG](CHANGELOG.md) for release history.

---

## License

[AGPL-3.0](LICENSE) — commercial licenses available for proprietary use.
Contact `sales@itunified.io`.
