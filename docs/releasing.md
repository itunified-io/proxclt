# Releasing proxctl

This runbook documents the multi-arch release pipeline for `proxctl`: binaries on GitHub Releases, Docker images on GHCR, and a Homebrew tap formula.

## Channels

1. **GitHub Releases** — multi-arch tarballs (linux/darwin × amd64/arm64) + `checksums.txt`
2. **GHCR** — `ghcr.io/itunified-io/proxctl:<version>` and `:latest` (linux/amd64 + linux/arm64 manifest)
3. **Homebrew tap** — formula pushed to `itunified-io/homebrew-tap`

## Prerequisites

- `goreleaser` ≥ 2.15 (`brew install goreleaser/tap/goreleaser`)
- Docker Desktop running with buildx (`docker buildx inspect --bootstrap`)
- `gh auth login` with `write:packages` scope (check `gh auth status`)
- `GITHUB_TOKEN` exported (GoReleaser uses it for release upload + tap push):
  ```bash
  export GITHUB_TOKEN=$(gh auth token)
  ```
- Docker logged into GHCR:
  ```bash
  echo "$GITHUB_TOKEN" | docker login ghcr.io -u itunified-buecheleb --password-stdin
  ```
- `itunified-io/homebrew-tap` repo exists (public). If missing:
  ```bash
  gh repo create itunified-io/homebrew-tap --public \
      --description "Homebrew tap for itunified-io tools"
  ```

## Important: CalVer tags need `--skip=validate`

The tags use **CalVer** (`v2026.04.11.7`), not SemVer. GoReleaser validates semver by default. Always pass `--skip=validate` for real releases, or it will abort with `invalid semantic version`.

## Dry-run (snapshot, no publish)

Validates config + builds binaries locally. No uploads. Artifacts land in `dist/`.

```bash
cd /path/to/proxctl
goreleaser release --clean --snapshot --skip=publish
ls dist/
```

## Real release

Assumes the tag `vYYYY.MM.DD.N` already exists on the remote and `HEAD` is at that tag.

```bash
cd /path/to/proxctl
git checkout v2026.04.11.7        # ensure HEAD == tag
export GITHUB_TOKEN=$(gh auth token)
echo "$GITHUB_TOKEN" | docker login ghcr.io -u itunified-buecheleb --password-stdin
goreleaser release --clean --skip=validate
```

GoReleaser will:

1. Cross-compile 4 binaries (linux/darwin × amd64/arm64)
2. Package tarballs + generate `checksums.txt`
3. Build 2 Docker images (amd64 + arm64) and push to GHCR
4. Create a GHCR manifest list for `:VERSION` and `:latest`
5. Upload all archives + checksums to the existing GitHub release
6. Generate `proxctl.rb` Homebrew formula and push to `itunified-io/homebrew-tap`

## Mandatory post-publish cleanup

Per infrastructure CLAUDE.md — after successful GHCR push, reclaim disk space:

```bash
docker image prune -f
docker buildx prune --keep-storage=2GB -f
```

## Verification

```bash
# Binaries on release page
gh release view v2026.04.11.7 --repo itunified-io/proxctl

# Docker image
docker pull ghcr.io/itunified-io/proxctl:v2026.04.11.7
docker run --rm ghcr.io/itunified-io/proxctl:v2026.04.11.7 version

# Homebrew tap
brew tap itunified-io/tap
brew install proxctl
proxctl version
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `invalid semantic version` | Add `--skip=validate` — CalVer is expected |
| `unauthorized: authentication required` (GHCR push) | Re-run `docker login ghcr.io` with a token that has `write:packages` |
| `403` pushing to homebrew-tap | Ensure the tap repo exists and `GITHUB_TOKEN` has `repo` scope |
| `git is currently in a dirty state` | Commit or stash changes; GoReleaser refuses dirty trees by default |
| Docker Desktop not responding | Start Docker.app, wait for whale icon, re-check `docker info` |

## Notes on deprecations (future work)

GoReleaser 2.x warns that `dockers` + `docker_manifests` will be replaced by `dockers_v2`, and `brews` by `homebrew_casks`. Migration is optional until v3. Track in a follow-up issue.
