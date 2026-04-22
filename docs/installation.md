# Installing proxctl

`proxctl` ships as a single static Go binary. No runtime dependencies apart from
`xorriso` (or `mkisofs`) on the machine that builds ISOs — the Proxmox side
needs nothing extra.

- [Supported platforms](#supported-platforms)
- [Homebrew (recommended for macOS/Linux)](#homebrew)
- [Direct binary download](#direct-binary-download)
- [Docker image](#docker-image)
- [Build from source](#build-from-source)
- [Air-gap install](#air-gap-install)
- [Shell completion](#shell-completion)
- [License setup](#license-setup)
- [Config directory layout](#config-directory-layout)
- [First-run verification](#first-run-verification)

---

## Supported platforms

| OS     | amd64 | arm64 |
|--------|-------|-------|
| linux  | yes   | yes   |
| darwin | yes   | yes   |
| windows| planned | planned |

proxctl requires **Go 1.23+** only if you build from source. Pre-built binaries
are statically linked (`CGO_ENABLED=0`) and work on any glibc or musl Linux
distribution.

---

## Homebrew

> The Homebrew tap is published alongside the first GoReleaser-built tag. Until
> then, use the [direct binary](#direct-binary-download) path below.

```bash
brew install itunified-io/proxctl/proxctl
proxctl version
```

To upgrade:

```bash
brew upgrade proxctl
```

---

## Direct binary download

Every GitHub Release publishes archives for `linux/darwin × amd64/arm64`:

```bash
# Linux amd64
curl -L -o /tmp/proxctl.tar.gz \
  https://github.com/itunified-io/proxctl/releases/latest/download/proxctl_Linux_x86_64.tar.gz
tar -xzf /tmp/proxctl.tar.gz -C /tmp
sudo install -m 0755 /tmp/proxctl /usr/local/bin/proxctl

# macOS (Apple Silicon)
curl -L -o /tmp/proxctl.tar.gz \
  https://github.com/itunified-io/proxctl/releases/latest/download/proxctl_Darwin_arm64.tar.gz
tar -xzf /tmp/proxctl.tar.gz -C /tmp
sudo install -m 0755 /tmp/proxctl /usr/local/bin/proxctl
```

Verify checksum:

```bash
curl -L -O https://github.com/itunified-io/proxctl/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

---

## Docker image

Multi-arch image available on GHCR:

```bash
docker pull ghcr.io/itunified-io/proxctl:latest
docker run --rm ghcr.io/itunified-io/proxctl:latest version
```

Mount your config directory to persist contexts + envs:

```bash
docker run --rm -it \
  -v $HOME/.proxctl:/root/.proxctl \
  -v $(pwd)/envs:/workspace \
  -w /workspace \
  ghcr.io/itunified-io/proxctl:latest workflow plan
```

Pin to a specific CalVer tag (`v2026.04.11.7`) for reproducible pipelines.

---

## Build from source

Requires **Go 1.23+**.

```bash
go install github.com/itunified-io/proxctl/cmd/proxctl@latest
proxctl version
```

Or clone + `make`:

```bash
git clone https://github.com/itunified-io/proxctl.git
cd proxctl
make build
./bin/proxctl version
```

Version metadata (`Version`, `Commit`, `Date`) is injected at build time via
`-ldflags`. See the [`Makefile`](../Makefile) for details.

---

## Air-gap install

For disconnected environments, download the full release tarball + checksum +
signature bundle on a connected host:

```bash
TAG=v2026.04.11.7
curl -L -O https://github.com/itunified-io/proxctl/releases/download/$TAG/proxctl_Linux_x86_64.tar.gz
curl -L -O https://github.com/itunified-io/proxctl/releases/download/$TAG/checksums.txt
curl -L -O https://github.com/itunified-io/proxctl/releases/download/$TAG/checksums.txt.sig

# Transfer all three files to the target host, then:
sha256sum --check --ignore-missing checksums.txt
tar -xzf proxctl_Linux_x86_64.tar.gz
sudo install -m 0755 proxctl /usr/local/bin/proxctl
```

The Enterprise tier additionally ships an **offline activation bundle** for
the license gate — see [licensing.md](licensing.md#offline-activation).

---

## Shell completion

proxctl ships Cobra-generated completion scripts.

### bash

```bash
proxctl completion bash | sudo tee /etc/bash_completion.d/proxctl >/dev/null
# or per-user:
proxctl completion bash > ~/.local/share/bash-completion/completions/proxctl
```

### zsh

```bash
mkdir -p ~/.zsh/completions
proxctl completion zsh > ~/.zsh/completions/_proxctl
# and ensure fpath contains ~/.zsh/completions before compinit in .zshrc:
#   fpath=(~/.zsh/completions $fpath)
#   autoload -Uz compinit && compinit
```

### fish

```bash
proxctl completion fish > ~/.config/fish/completions/proxctl.fish
```

### PowerShell

```powershell
proxctl completion powershell | Out-String | Invoke-Expression
# persist by appending the output to $PROFILE
```

---

## License setup

proxctl runs in **Community mode** with no license file — the core workflow
(`config`, `env`, `vm`, `snapshot`, `kickstart`, `boot`, `workflow` serial)
works out of the box under AGPL-3.0.

Business and Enterprise features require a JWT license. Three supply paths,
checked in this order:

1. `--license /path/to/license.jwt` flag
2. `PROXCTL_LICENSE` environment variable (raw JWT string **or** file path)
3. `~/.proxctl/license.jwt` file (default)

```bash
# Place the license file
mkdir -p ~/.proxctl
install -m 0600 /path/to/license.jwt ~/.proxctl/license.jwt

# Or via env var (pipeline-friendly)
export PROXCTL_LICENSE="$(cat /secret/mount/license.jwt)"

proxctl license show
proxctl license status
```

Full details: [licensing.md](licensing.md).

---

## Config directory layout

proxctl stores state under `~/.proxctl/` by default. Override with
`$PROXCTL_HOME`:

```
~/.proxctl/
├── config.yaml          # kubectl-style contexts (Proxmox endpoints + tokens)
├── stacks.yaml            # registered env bookmarks
├── license.jwt          # optional license file
├── profiles/            # user profile overrides (optional)
│   └── my-profile.yaml
├── state.db             # SQLite state (VM inventory, snapshot history)
└── audit.log            # append-only operator action log
```

Create the directory up front to get the right permissions:

```bash
mkdir -p ~/.proxctl
chmod 700 ~/.proxctl
```

`config.yaml` is written by `proxctl config use-context`. Example:

```yaml
current-context: lab-pve
contexts:
  - name: lab-pve
    endpoint: https://pve.lab.example.com:8006/api2/json
    token_id: root@pam!proxctl
    token_secret_ref: "${env:PVE_TOKEN_SECRET}"
    insecure_tls: false
```

---

## First-run verification

After installing, run:

```bash
proxctl version
#   proxctl v2026.04.11.7  commit=abcdef0  date=2026-04-22T10:15:00Z  go=go1.23.4

proxctl --help

# Register a Proxmox context (interactive)
proxctl config use-context lab-pve \
  --endpoint https://pve.lab.example.com:8006/api2/json \
  --token-id 'root@pam!proxctl' \
  --token-secret "$PVE_TOKEN_SECRET"

proxctl config get-contexts
proxctl config current-context
```

If `proxctl config current-context` prints `lab-pve`, you are ready to follow
the [quick-start](quick-start.md).
