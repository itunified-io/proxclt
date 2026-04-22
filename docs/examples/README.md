# Example envs

Three complete, validated envs demonstrating each shipped profile. Each
directory contains the full split-file manifest (lab.yaml + layer files)
plus a `linux.yaml` passthrough.

| Directory | Profile | Use case |
|-----------|---------|----------|
| [`host-only/`](host-only) | `host-only` | Single Linux VM, no cluster |
| [`pg-single/`](pg-single) | `pg-single` | Single-node PostgreSQL |
| [`oracle-rac-2node/`](oracle-rac-2node) | `oracle-rac-2node` | 2-node Oracle RAC |

Every example validates via:

```bash
proxctl config validate docs/examples/<name>/lab.yaml
```

Secrets are referenced via `${env:...}` (Proxmox API token) or
`${vault:...}` (cluster passwords). Set the env vars or stub them out with
the `| default=...` filter before validating.
