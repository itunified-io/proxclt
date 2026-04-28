package root

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/itunified-io/proxctl/pkg/config"
	"github.com/itunified-io/proxctl/pkg/proxmox"
	"gopkg.in/yaml.v3"
)

// proxctlConfig is the minimal on-disk config at ~/.proxctl/config.yaml.
// (kubectl-style contexts — Phase 2 uses only a single current context.)
type proxctlConfig struct {
	CurrentContext string           `yaml:"current-context"`
	Contexts       []proxctlContext `yaml:"contexts"`
}

type proxctlContext struct {
	Name        string `yaml:"name"`
	Endpoint    string `yaml:"endpoint"`
	TokenID     string `yaml:"token_id"`
	TokenSecret string `yaml:"token_secret"`
	InsecureTLS bool   `yaml:"insecure_tls"`
}

// loadProxmoxClient loads credentials from ~/.proxctl/config.yaml (if present)
// or from env vars PROXCTL_ENDPOINT / PROXCTL_TOKEN_ID / PROXCTL_TOKEN_SECRET.
func loadProxmoxClient() (*proxmox.Client, error) {
	var (
		endpoint    = os.Getenv("PROXCTL_ENDPOINT")
		tokenID     = os.Getenv("PROXCTL_TOKEN_ID")
		tokenSecret = os.Getenv("PROXCTL_TOKEN_SECRET")
		insecure    = os.Getenv("PROXCTL_INSECURE_TLS") == "1"
	)

	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".proxctl", "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		var cfg proxctlConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", cfgPath, err)
		}
		name := flagContext
		if name == "" {
			name = cfg.CurrentContext
		}
		for _, c := range cfg.Contexts {
			if c.Name == name {
				if endpoint == "" {
					endpoint = c.Endpoint
				}
				if tokenID == "" {
					tokenID = c.TokenID
				}
				if tokenSecret == "" {
					tokenSecret = c.TokenSecret
				}
				insecure = insecure || c.InsecureTLS
				break
			}
		}
	}

	if endpoint == "" || tokenID == "" || tokenSecret == "" {
		return nil, errors.New("proxmox credentials missing: set $PROXCTL_ENDPOINT/$PROXCTL_TOKEN_ID/$PROXCTL_TOKEN_SECRET or create ~/.proxctl/config.yaml")
	}
	return proxmox.NewClient(proxmox.ClientOpts{
		Endpoint:    endpoint,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		InsecureTLS: insecure,
	})
}

// loadEnvManifest loads an env manifest from disk. It looks first at flagStack
// (the --stack path; the deprecated --env flag is folded into flagStack by
// resolveDeprecatedFlags), then at ./env.yaml.
//
// Routes through config.Load so $ref pointers are resolved, profile extends are
// applied, and secret placeholders are expanded — the same path
// `proxctl config render` uses. Without this, callers' subsequent calls to
// `env.Spec.Hypervisor.Resolved()` return nil with "hypervisor not resolved"
// on any manifest that composes via $ref (the canonical pattern in
// infrastructure/stacks/<stack>/env.yaml). See #19.
//
// NOTE: the on-disk filename stays `env.yaml` and the Go type stays
// `config.Env`. Only the CLI verb/flag changed in #15. See CHANGELOG
// v2026.04.11.8.
func loadEnvManifest(explicitPath string) (*config.Env, error) {
	path := explicitPath
	if path == "" {
		path = flagStack
	}
	if path == "" {
		path = "env.yaml"
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	env, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return env, nil
}

// resolveNodeRef pulls the ProxmoxRef (node name + vmid) for the given node name.
func resolveNodeRef(env *config.Env, nodeName string) (pveNode string, vmid int, err error) {
	hyp := env.Spec.Hypervisor.Resolved()
	if hyp == nil {
		return "", 0, errors.New("hypervisor not resolved")
	}
	n, ok := hyp.Nodes[nodeName]
	if !ok {
		return "", 0, fmt.Errorf("node %q not in manifest", nodeName)
	}
	return n.Proxmox.NodeName, n.Proxmox.VMID, nil
}
