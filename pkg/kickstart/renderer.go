// Package kickstart renders per-node kickstart/preseed files from the
// proxctl env manifest and bundles them into bootable ISOs.
package kickstart

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"sort"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/itunified-io/proxctl/pkg/config"
)

//go:embed templates/common/*.tmpl templates/*/*.tmpl
var templatesFS embed.FS

// Renderer loads and executes the bundled kickstart templates.
type Renderer struct {
	// distro -> entry-point template ("base.ks" / "preseed.cfg")
	entries map[string]*template.Template
	distros []string
}

// RenderContext is the data struct passed to every template.
type RenderContext struct {
	Env       *config.Env
	Node      *config.Node
	NodeName  string
	Hostname  string
	FQDN      string
	Kickstart *config.KickstartConfig
	Networks  map[string]config.NetworkZone
	Cluster   *config.Cluster
	PublicIP  string
	VIPIP     string
	PrivateIP string
	ScanIPs   []string
}

// NewRenderer loads all bundled templates.
func NewRenderer() (*Renderer, error) {
	return newRendererFromFS(templatesFS)
}

// newRendererFromFS is the injectable-FS variant of NewRenderer, used by tests
// to exercise error paths around missing/corrupt templates without touching
// the embedded filesystem.
func newRendererFromFS(tfs fs.FS) (*Renderer, error) {
	r := &Renderer{entries: map[string]*template.Template{}}

	// Discover distro directories under templates/.
	entries, err := fs.ReadDir(tfs, "templates")
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	// Load common partials (we parse them into each distro's template set).
	commonFiles, _ := fs.Glob(tfs, "templates/common/*.tmpl")

	funcs := funcMap()

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "common" {
			continue
		}
		distro := e.Name()
		distroFiles, err := fs.Glob(tfs, "templates/"+distro+"/*.tmpl")
		if err != nil {
			return nil, err
		}
		if len(distroFiles) == 0 {
			continue
		}
		t := template.New(distro).Funcs(funcs)
		// Parse common partials first.
		if len(commonFiles) > 0 {
			if _, err := t.ParseFS(tfs, commonFiles...); err != nil {
				return nil, fmt.Errorf("parse common: %w", err)
			}
		}
		if _, err := t.ParseFS(tfs, distroFiles...); err != nil {
			return nil, fmt.Errorf("parse distro %s: %w", distro, err)
		}
		r.entries[distro] = t
		r.distros = append(r.distros, distro)
	}
	sort.Strings(r.distros)
	return r, nil
}

// SupportedDistros returns the list of distro names bundled with this binary.
func (r *Renderer) SupportedDistros() []string {
	out := make([]string, len(r.distros))
	copy(out, r.distros)
	return out
}

// Render generates a kickstart file for nodeName using env's hypervisor/kickstart data.
func (r *Renderer) Render(env *config.Env, nodeName string) (string, error) {
	if env == nil {
		return "", fmt.Errorf("env is nil")
	}

	hyp := env.Spec.Hypervisor.Resolved()
	if hyp == nil {
		return "", fmt.Errorf("env.spec.hypervisor not resolved")
	}
	node, ok := hyp.Nodes[nodeName]
	if !ok {
		return "", fmt.Errorf("node %q not found in hypervisor.nodes", nodeName)
	}
	if hyp.Kickstart == nil {
		return "", fmt.Errorf("env has no kickstart config")
	}
	ks := hyp.Kickstart
	distro := ks.Distro
	if distro == "" {
		return "", fmt.Errorf("kickstart.distro is empty")
	}
	t, ok := r.entries[distro]
	if !ok {
		return "", fmt.Errorf("unsupported distro %q (supported: %v)", distro, r.distros)
	}

	nets := map[string]config.NetworkZone{}
	if env.Spec.Networks.Resolved() != nil {
		nets = env.Spec.Networks.Resolved().Zones
	}
	var cluster *config.Cluster
	if env.Spec.Cluster != nil {
		cluster = env.Spec.Cluster.Resolved()
	}

	hostname := nodeName
	domain := env.Metadata.Domain
	fqdn := hostname
	if domain != "" {
		fqdn = hostname + "." + domain
	}

	ctx := RenderContext{
		Env:       env,
		Node:      &node,
		NodeName:  nodeName,
		Hostname:  hostname,
		FQDN:      fqdn,
		Kickstart: ks,
		Networks:  nets,
		Cluster:   cluster,
		PublicIP:  node.IPs["public"],
		VIPIP:     node.IPs["vip"],
		PrivateIP: node.IPs["private"],
	}
	if cluster != nil {
		ctx.ScanIPs = cluster.ScanIPs
	}

	// Pick entry-point template. Prefer a template named "base.ks" then "preseed.cfg".
	entry := pickEntry(t)
	if entry == "" {
		return "", fmt.Errorf("distro %q: no entry template (expected base.ks.tmpl or preseed.cfg.tmpl)", distro)
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, entry, ctx); err != nil {
		return "", fmt.Errorf("execute %s/%s: %w", distro, entry, err)
	}
	return buf.String(), nil
}

// pickEntry chooses the entry-point template name from the parsed set.
func pickEntry(t *template.Template) string {
	candidates := []string{"base.ks.tmpl", "preseed.cfg.tmpl", "base.ks", "preseed.cfg"}
	for _, name := range candidates {
		if t.Lookup(name) != nil {
			return name
		}
	}
	return ""
}

// funcMap returns the template function map with sprig + custom helpers.
func funcMap() template.FuncMap {
	fm := sprig.TxtFuncMap()
	fm["cidrIP"] = cidrIP
	fm["cidrNetmask"] = cidrNetmask
	fm["cidrPrefix"] = cidrPrefix
	fm["joinLines"] = joinLines
	return fm
}

// cidrIP returns the host IP part of a CIDR like "10.10.0.100/24" -> "10.10.0.100".
func cidrIP(s string) string {
	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		// Fallback: accept bare IP.
		if parsed := net.ParseIP(s); parsed != nil {
			return parsed.String()
		}
		return s
	}
	return ip.String()
}

// cidrNetmask returns the dotted-quad netmask for a CIDR (IPv4).
func cidrNetmask(s string) string {
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return ""
	}
	mask := ipnet.Mask
	if len(mask) == 4 {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	// IPv6 — return prefix length instead.
	ones, _ := mask.Size()
	return fmt.Sprintf("/%d", ones)
}

// cidrPrefix returns the prefix length for a CIDR.
func cidrPrefix(s string) string {
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return ""
	}
	ones, _ := ipnet.Mask.Size()
	return fmt.Sprintf("%d", ones)
}

// joinLines joins a slice with newlines, trimming empties.
func joinLines(ss []string) string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		s = strings.TrimRight(s, "\n")
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}
