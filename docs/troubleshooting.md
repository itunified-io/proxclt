# Troubleshooting

Common failure modes and how to resolve them. Every subcommand supports
`--verbose` to emit HTTP traces, kickstart inputs, and workflow state
transitions.

General tools:

- `proxctl --verbose <cmd>` — structured logs to stderr
- `~/.proxctl/audit.log` — append-only operator action log
- `proxctl config render <env.yaml>` — show the fully resolved manifest
- `proxctl config validate <env.yaml>` — cross-file invariant check

---

## 1. Proxmox auth: 401 Unauthorized

**Symptom.** Any VM/storage/ISO call fails with `401 authentication failure`.

**Cause.** Token id or secret wrong, token disabled, or token has no
permissions on the target path.

**Fix.**

```bash
# Re-test credentials
curl -sk -H "Authorization: PVEAPIToken=$PVE_TOKEN_ID=$PVE_TOKEN_SECRET" \
  https://pve.lab.example.com:8006/api2/json/version

# Grant the token VM.Allocate, Datastore.AllocateSpace, etc. in the Proxmox UI
# Re-register the context
proxctl config use-context lab-pve --token-secret "$NEW_SECRET"
```

## 2. Proxmox auth: token expired

**Symptom.** `401 token expired or invalidated`.

**Cause.** Token TTL exceeded or token rotated.

**Fix.** Generate a new token in Proxmox UI (Datacenter → Permissions → API
Tokens), then `proxctl config use-context <name> --token-secret "$NEW"`.

## 3. InsecureTLS required

**Symptom.** `x509: certificate signed by unknown authority`.

**Cause.** Proxmox is using a self-signed cert (default).

**Fix.** Either install the Proxmox CA into the system trust store, or
register the context with `--insecure-tls` (not recommended for prod).

## 4. ISO upload: storage full

**Symptom.** `kickstart upload` returns `507 insufficient storage`.

**Cause.** Destination storage has no free space.

**Fix.** `proxctl kickstart upload` also cleans up stale first-boot ISOs
that match `proxctl-ks-*`. If the storage is genuinely full, free space on
the Proxmox host (`pvesm status`, `pvesm free`).

## 5. ISO upload: permission denied

**Symptom.** `403 Permission check failed (/storage/<name>, Datastore.AllocateSpace)`.

**Cause.** API token missing `Datastore.AllocateSpace` on the ISO storage.

**Fix.** Grant the permission in Proxmox UI → Datacenter → Permissions → Add.

## 6. Kickstart boot: DNS resolution fails

**Symptom.** Install stops at "Unable to download package metadata".

**Cause.** The first-boot ISO used DNS but `kickstart.chrony_servers` or the
NIC's `dns:` list is unreachable.

**Fix.** Either add working DNS to `networks.yaml:<zone>.dns` (inherited by
all NICs) or hardcode resolvers under the NIC's `ipv4.dns:`. Re-run
`proxctl kickstart generate` to preview the rendered config.

## 7. Kickstart boot: mirror unreachable

**Symptom.** Install stops at "Base repository not found" (OL/RHEL) or
"failed to download Release" (Ubuntu).

**Cause.** The VM cannot reach the distro mirror. Usually a firewall rule
between the `public` zone and the upstream.

**Fix.** Confirm the VM got an IP (`proxctl workflow status`), ping the
mirror from the Proxmox host's network, or mirror the packages locally and
override the template.

## 8. VM stuck in "creating"

**Symptom.** `proxctl workflow status` shows `state=creating` for >5 min.

**Cause.** Proxmox task hung on ISO attach or storage provisioning.

**Fix.**

```bash
# Find the hung task
proxctl --verbose vm status <node> --env <env>

# In Proxmox UI → Node → Tasks, kill the stuck task
# Then:
proxctl vm delete <node> --env <env> --yes
proxctl workflow up --env <env> --yes   # idempotent; recreates cleanly
```

## 9. SSH reachability fails after install

**Symptom.** `workflow verify` times out with `ssh: no reachable address`.

**Cause.** VM booted but the kickstart did not install `qemu-guest-agent`
(proxctl queries IPs via the agent), or firewall blocks port 22, or the
public NIC came up on a different zone than expected.

**Fix.**

- Add `qemu-guest-agent` to `kickstart.packages.base`.
- Confirm `kickstart.firewall.enabled: false` (or explicitly allow 22).
- Check the VM console from Proxmox UI — confirm the actual IP matches
  `hypervisor.yaml:nodes.<n>.ips.public`.

## 10. License gate errors

**Symptom.** `license required: workflow.up --parallel requires the Business tier`.

**Cause.** Calling a gated feature without a valid Business/Enterprise JWT.

**Fix.** See [licensing.md](licensing.md). Either drop the gated flag
(`--parallel 1` is free) or place a valid license at
`~/.proxctl/license.jwt`.

## 11. Config validation: unknown field

**Symptom.** `field foo not found in type config.Node`.

**Cause.** Typo or a key that was removed between versions.

**Fix.** Check [config-reference.md](config-reference.md) for the current
schema. `proxctl config validate` prints the exact path (`spec.hypervisor.nodes.rac-node-1.foo`).

## 12. Config validation: $ref not found

**Symptom.** `ref ./hypervisor.yaml: open: no such file or directory`.

**Cause.** Refs are resolved **relative to the file that contains them**,
not relative to your shell's cwd.

**Fix.** Run `proxctl config validate <full-path-to-lab.yaml>`, not from a
subdirectory. Refs with absolute paths always work.

## 13. Secret resolver: vault unreachable

**Symptom.** `vault resolver: dial tcp: connection refused`.

**Cause.** `VAULT_ADDR` unset or Vault sealed.

**Fix.**

```bash
export VAULT_ADDR=https://vault.example.com:8200
export VAULT_TOKEN="$(cat ~/.vault-token)"
proxctl config render <env.yaml>    # retry
```

Or switch the placeholder to `${env:...}` / `${file:...}` for environments
without Vault.

## 14. Secret resolver: env var unset

**Symptom.** `env resolver: PVE_TOKEN_SECRET not set`.

**Cause.** Placeholder references an unset variable and no `default=` filter.

**Fix.** Either `export PVE_TOKEN_SECRET=...` before the command, or amend
the placeholder to `${env:PVE_TOKEN_SECRET | default=placeholder}`.

## 15. Multi-node concurrency: duplicate ISO uploads

**Symptom.** Same ISO uploaded multiple times; or `409 file already exists`.

**Cause.** Two parallel workflows targeting the same storage without
coordination — should not happen within one `proxctl workflow up`, but can
happen across concurrent CLI invocations.

**Fix.** Within one env the mutex in `MultiNodeWorkflow` serialises
uploads. Across envs, run serially or target different `kickstart_storage`
values.

## 16. Workflow rollback: orphaned disks

**Symptom.** After a failed `up`, `pvesm list` shows disks that were created
but not attached to a VM.

**Cause.** Proxmox created the disk, then the VM-create task failed after,
and rollback could not reach the disk-delete step.

**Fix.** `proxctl workflow down` is idempotent and scans for orphaned
resources matching the env's VMID range. Run it once; then re-apply.

## 17. Audit log: stale entries

**Symptom.** Entries from a previous operator / machine.

**Cause.** The audit log is append-only and not auto-rotated.

**Fix.** `logrotate` it manually; Enterprise tier offers a hash-chain
verifier that rejects rotated logs unless signed.

## 18. Permission denied on ~/.proxctl/state.db

**Symptom.** `sqlite: unable to open database file`.

**Cause.** Running proxctl under a different user than the one that created
`~/.proxctl/`.

**Fix.** `chown -R $USER ~/.proxctl && chmod 700 ~/.proxctl`.

## 19. "extends: oracle-rac-2node" but nothing inherited

**Symptom.** `workflow plan` still complains about missing defaults that
the profile should supply.

**Cause.** Typo in the profile name, or a user-supplied profile at
`~/.proxctl/profiles/oracle-rac-2node.yaml` with the same name
**overriding** the shipped one (user profiles always win).

**Fix.** `proxctl workflow profile show oracle-rac-2node` prints the
effective content — if it is empty, remove the shadowing user profile.

## 20. Proxmox API rate limits

**Symptom.** `429 too many requests` on large envs.

**Cause.** Very large (>20 node) envs with `--parallel` > 4 saturate the
Proxmox API.

**Fix.** Drop `--parallel` to 2–4 and retry. There is no backoff tuning knob
yet — track [issue #TBD](https://github.com/itunified-io/proxctl/issues) for
adaptive rate limiting.
