# Licensing

proxctl follows the same three-tier commercial model as the rest of the dbx
platform. The core provisioning workflow is free and AGPL-licensed; advanced
operational features are gated via a signed JWT license.

- [Tier model](#tier-model)
- [Feature matrix](#feature-matrix)
- [Obtaining a license](#obtaining-a-license)
- [License file, env var, flag](#license-file-env-var-flag)
- [Grace period](#grace-period)
- [Offline activation](#offline-activation)
- [Seat counting](#seat-counting)
- [Pricing anchors](#pricing-anchors)
- [Bundles](#bundles)

---

## Tier model

| Tier | License | Scope |
|------|---------|-------|
| **Community** | AGPL-3.0 | Full VM provisioning workflow. No time-bomb, no feature gate. |
| **Business**  | Commercial (per-seat) | Profile CRUD, drift detection, parallel workflows, REST API, priority support. |
| **Enterprise**| Commercial (custom)   | Everything in Business plus audit hash-chain, central state sync, RBAC, air-gapped activation bundles, SLA. |

The license gate is implemented in
[`pkg/license/gate.go`](../pkg/license/gate.go). Every leaf subcommand calls
`license.Check("<tool-name>")` before doing work; the authoritative
`ToolCatalog` map in that file is the source of truth for which command
belongs to which tier.

---

## Feature matrix

| Feature | Community | Business | Enterprise |
|---------|:---------:|:--------:|:----------:|
| `config validate / render / use-context` | ✓ | ✓ | ✓ |
| `env new / list / use / add / remove`    | ✓ | ✓ | ✓ |
| `vm create / start / stop / delete / list / status` | ✓ | ✓ | ✓ |
| `snapshot create / restore / list / delete` | ✓ | ✓ | ✓ |
| `kickstart generate / build-iso / upload / distros` | ✓ | ✓ | ✓ |
| `boot configure-first-boot / eject-iso`  | ✓ | ✓ | ✓ |
| `workflow plan / up / down / status / verify` (serial) | ✓ | ✓ | ✓ |
| `workflow up --parallel N` (N > 1)       |   | ✓ | ✓ |
| `workflow up --continue-on-error`        |   | ✓ | ✓ |
| `workflow profile` CRUD (create/edit via CLI) |   | ✓ | ✓ |
| Drift detection (`workflow diff`, scheduled) |   | ✓ | ✓ |
| REST API + webhooks                      |   | ✓ | ✓ |
| Audit log (plain append)                 | ✓ | ✓ | ✓ |
| Audit log **hash-chain + export**        |   |   | ✓ |
| Central state sync (S3/Vault backend)    |   |   | ✓ |
| RBAC (operator roles + per-env policies) |   |   | ✓ |
| Air-gapped offline activation bundle     |   |   | ✓ |
| SAML / OIDC seat federation              |   |   | ✓ |
| SLA (response time, uptime)              |   |   | ✓ |

Community stays fully functional for lab and small-team use. The CLI never
silently degrades — if you invoke a gated feature without a license, you
get an explicit `license required: <feature> requires the Business tier`
error.

---

## Obtaining a license

1. Trial: email **sales@itunified.io** for a 30-day signed trial JWT.
   Trials include the full Enterprise feature set for evaluation.
2. Business: purchase seats at `€99 / operator / month` (annual) via the
   iTunified customer portal. A JWT is issued with your seat allocation and
   expiry.
3. Enterprise: custom contract (starts at `€25,000 / year`) — contact
   **sales@itunified.io** for scoping.

The JWT embeds:

- Tier (`community` / `business` / `enterprise`)
- Seat count
- Expiry (`exp`)
- Issuer / subject / audience (`itunified.io` / customer / `proxctl`)
- Feature flags for Enterprise (audit hash-chain, state-sync backend id, …)

It is signed with an Ed25519 key whose public key is pinned in the proxctl
binary — tampering invalidates the signature at the next `license.Check()`.

---

## License file, env var, flag

Supply the JWT via any of (checked in this order):

1. `--license /path/to/license.jwt` flag on any subcommand.
2. `PROXCTL_LICENSE` environment variable. May be either the raw JWT string
   or a path to a file containing the JWT.
3. `~/.proxctl/license.jwt` file (default). Permissions must be `0600` or
   stricter.

```bash
install -m 0600 license.jwt ~/.proxctl/license.jwt
proxctl license show      # prints tier, seats, expiry
proxctl license status    # prints validation status (active / grace / expired / invalid)
proxctl license activate --license /path/to/new-license.jwt
proxctl license seats-used
```

`license show` never prints the raw JWT — only the decoded claims.

---

## Grace period

Business and Enterprise licenses have a **14-day grace period** after the
expiry date. During grace:

- Every subcommand continues to work.
- stderr emits `warning: license expired, 14 days grace remaining`.
- `license status` prints `expired-grace` with the remaining count.

After grace expires, gated features begin to fail with `license expired`.
Community-tier commands never fail — even an expired Business license still
lets you run the free subset.

The grace period exists so a missed renewal never causes a production
outage during business hours.

---

## Offline activation

Enterprise tier only. For air-gapped environments:

1. Your license issuer generates an **offline activation bundle**:
   ```
   proxctl-offline-bundle-<customer>-<year>.tar.gz
   ├── license.jwt
   ├── seats.db              # pre-seeded seat identities
   └── issuer-fingerprint.txt
   ```
2. Transfer the bundle to the air-gapped host.
3. `proxctl license activate --bundle /path/to/bundle.tar.gz`
4. proxctl unpacks the bundle into `~/.proxctl/`.

Offline-bundle seats are tracked locally in `seats.db`; no network check is
ever attempted. Seat reports are exported via
`proxctl license seats-used --json > seats.json` for periodic reconciliation.

---

## Seat counting

A **seat** is a distinct **operator identity** seen by the license gate in a
rolling 30-day window. The operator identity is the OS username combined
with the machine's Linux `/etc/machine-id` (or macOS `IOPlatformUUID`),
hashed.

- Running proxctl on five laptops under the same username = **five seats**.
- Running it in 500 CI pipelines under the same GitHub runner
  image = **one seat** (same machine-id).
- Seats do not decrement — if you lay off an operator, the seat is reclaimed
  after 30 days of inactivity.

`proxctl license seats-used` prints the current count and the cap from your
JWT.

Exceeding the cap is a **soft gate**: proxctl warns loudly and keeps
running, but continued overage for more than 30 days will fail gated
commands with `seat limit exceeded`.

---

## Pricing anchors

| Tier | Public anchor |
|------|---------------|
| Community  | Free (AGPL) |
| Business   | **€99 / operator seat / month**, billed annually, 10-seat minimum |
| Enterprise | **from €25,000 / year**, includes 25 seats, SLA, and offline bundles |

Discounts for multi-year and academic use — ask sales.

---

## Bundles

proxctl is part of the dbx infrastructure platform. Bundle pricing available
when combined with `linuxctl` (Linux configuration management) and `dbx`
(database provisioning):

| Bundle | Components | Anchor |
|--------|------------|--------|
| Core   | proxctl Business + linuxctl Business     | 15% off list |
| Full   | proxctl + linuxctl + dbx (all Business)  | 25% off list |
| Enterprise Stack | All three at Enterprise         | custom; includes joint roadmap |

Licenses are cross-issued — one JWT can unlock all three tools if you hold
the bundle. Components keep their individual license gates so partial
deployments still work.
