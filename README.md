# proxctl

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8.svg)](https://go.dev/)

**proxctl** is a single-binary Go CLI for Proxmox VM provisioning — kickstart rendering, ISO remastering, VM lifecycle, snapshots, and multi-VM workflow orchestration.

> Status: **Phase 1 scaffold.** Command tree + license gate + SQLite state are stubs. Real implementation lands in Phases 2–5. See [design doc 024](https://github.com/itunified-io/infrastructure/blob/main/docs/plans/024-proxctl-design.md) (private) for the roadmap.

## Install

See [`docs/installation.md`](docs/installation.md) for details. Quick options:

```bash
# Homebrew (post-launch)
brew install itunified-io/tap/proxctl

# Direct binary
curl -L https://github.com/itunified-io/proxctl/releases/latest/download/proxctl-$(uname -s)-$(uname -m).tar.gz | tar xz

# Build from source
git clone https://github.com/itunified-io/proxctl.git && cd proxctl && make build
./bin/proxctl version
```

## Quick start

```bash
proxctl --help
proxctl version
proxctl config get-contexts
proxctl env list
```

Full user guide: [`docs/user-guide.md`](docs/user-guide.md).

## Tiers

| Tier | Price | Includes |
|------|-------|----------|
| Community | Free (AGPL) | config, env, vm, snapshot, kickstart, boot, workflow (serial), license |
| Business | €99/mo/seat | profile + template CRUD, drift detection, REST API, parallel workflows |
| Enterprise | Custom | audit hash-chain, central state sync, RBAC, air-gapped bundles |

See [`docs/licensing.md`](docs/licensing.md).

## License

[AGPL-3.0](LICENSE) — commercial licenses available for proprietary use.
