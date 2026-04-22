# Contributing

Thanks for your interest in proxctl. This page covers dev environment setup,
the test approach, coverage expectations, and the release flow.

- [Dev environment](#dev-environment)
- [Test approach](#test-approach)
- [Coverage targets](#coverage-targets)
- [Branch naming + PR flow](#branch-naming--pr-flow)
- [Release process](#release-process)

---

## Dev environment

Requirements:

- **Go 1.23+**
- **GNU Make** (optional, all commands also work raw)
- **xorriso** (or `mkisofs`) for integration tests that build real first-boot ISOs
- **staticcheck** — `go install honnef.co/go/tools/cmd/staticcheck@latest`

Clone + build:

```bash
git clone https://github.com/itunified-io/proxctl.git
cd proxctl
go mod download
make build              # → bin/proxctl
./bin/proxctl version
```

Everyday loop:

```bash
make test               # go test ./...
make vet                # go vet ./...
make staticcheck        # honnef.co/go/tools
make lint               # vet + staticcheck
```

---

## Test approach

Three tiers:

1. **Unit** — default build tag, runs on every commit. No network, no
   xorriso, no Proxmox. Uses `httptest` for the Proxmox client and
   in-memory FS (`fs.FS` → `fstest.MapFS`) for the kickstart renderer.
2. **Integration** — build tag `integration`. Runs against a live Proxmox
   test cluster; requires `PROXCTL_IT_ENDPOINT`, `PROXCTL_IT_TOKEN_ID`,
   `PROXCTL_IT_TOKEN_SECRET` env vars. Skipped by default.
3. **Race** — `go test -race ./...` on every PR; asserts the multi-node
   workflow's ISO upload mutex actually serialises.

Golden-file tests drive kickstart template output: change the template,
re-run with `-update`, inspect the diff, commit.

```bash
# Unit
go test ./...

# With race detector
go test -race ./...

# Integration (requires env vars)
go test -tags=integration ./pkg/proxmox/...
```

---

## Coverage targets

proxctl enforces **≥95% coverage** on all substantial packages. Current
floor (see the CHANGELOG for history):

| Package                  | Target | Status  |
|--------------------------|--------|---------|
| `pkg/config`             | ≥95%   | 95.1%   |
| `pkg/proxmox`            | ≥95%   | 97.5%   |
| `pkg/kickstart`          | ≥95%   | 97.6%   |
| `pkg/workflow`           | ≥95%   | 96.4%   |
| `pkg/license`            | ≥95%   | 100.0%  |
| `internal/root`          | ≥95%   | 95.0%   |
| `cmd/proxctl`            | n/a    | 0% (main shim) |
| `pkg/state`              | grows in Phase 6 | 0% (scaffold stub) |

CI fails if coverage on any `≥95%` package drops below the target. Bring
coverage up **with the change** — tests are not a follow-up.

Coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
go tool cover -html=coverage.out -o coverage.html
```

---

## Branch naming + PR flow

- Branches: `feature/<issue-nr>-<slug>`, `fix/<issue-nr>-<slug>`,
  `docs/<issue-nr>-<slug>`, `chore/<slug>`.
- Every change ties to a GitHub issue — reference it in commits
  (`feat: add parallel workflow support (#12)`) and PR descriptions
  (`Closes #12`).
- PR must be green on: `build`, `vet`, `staticcheck`, `test`, `test-race`,
  coverage gate.
- Every user-visible change updates `CHANGELOG.md` in the same PR.
- Squash on merge. Retain the PR number in the squash commit subject so
  the history is greppable.

For significant changes, an issue-linked design note in
`itunified-io/infrastructure` (private) or a markdown doc in this repo's
`docs/` is expected before implementation begins.

---

## Release process

proxctl uses CalVer (`vYYYY.MM.DD.TS`). Release on merge to `main`:

1. **Update CHANGELOG.md.** New section at the top with today's tag and a
   list of changes + issue refs.
2. **Commit** on main (or the merge commit if the PR already bumped the
   changelog).
3. **Tag**:
   ```bash
   TAG=v2026.04.22.1
   git tag -a "$TAG" -m "$TAG: <one-line summary>"
   git push origin --tags
   ```
4. **GoReleaser** runs via GitHub Actions on the tag push. It builds
   `linux/darwin × amd64/arm64` archives, generates checksums, pushes the
   Docker image to `ghcr.io/itunified-io/proxctl`, and updates the
   Homebrew tap (`itunified-io/homebrew-proxctl`).
5. **GitHub Release** is auto-created from the CHANGELOG entry.
6. **Verify**: `brew upgrade proxctl && proxctl version`.

Before tagging, always `git tag -l 'v2026.04.22.*'` to check for collisions
with same-day releases — the `.TS` suffix increments when more than one
release lands in a day.

---

## Coding conventions

- Package comments on every `package` clause.
- Exported identifiers get Go doc comments in the `// Foo bar baz` style.
- Errors are created with `fmt.Errorf("context: %w", err)`; callers use
  `errors.Is` / `errors.As`.
- No panics outside of `init()` and test helpers; return an error instead.
- Keep `pkg/*` free of CLI-layer concerns (no Cobra, no stdout printing) —
  domain packages take context + inputs and return values + errors.
- Test packages use `_test` suffix (`package config_test`) when they want
  to exercise the public API; same-package tests are fine for testing
  unexported helpers.
