package kickstart

import (
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/itunified-io/proxctl/pkg/config"
)

func buildEnv() *config.Env {
	node := config.Node{
		Proxmox: config.ProxmoxRef{NodeName: "pve01", VMID: 101},
		IPs:     map[string]string{"public": "10.10.0.100", "private": "10.11.0.100"},
		Resources: &config.Resources{
			Memory:  4096,
			Cores:   2,
			Sockets: 1,
			BIOS:    "ovmf",
		},
		NICs: []config.NIC{
			{
				NameField: "eth0",
				Bridge:    "vmbr0",
				Usage:     "public",
				Bootproto: "static",
				IPv4: &config.IPv4Config{
					Address: "10.10.0.100/24",
					Gateway: "10.10.0.1",
					DNS:     []string{"1.1.1.1"},
				},
			},
		},
		Disks: []config.Disk{
			{ID: 0, Size: "32G", Storage: "local-lvm", Interface: "scsi0"},
		},
	}

	hyp := &config.Hypervisor{
		Kind:  "Hypervisor",
		Nodes: map[string]config.Node{"web01": node},
		Kickstart: &config.KickstartConfig{
			Distro:         "oraclelinux9",
			Timezone:       "Europe/Berlin",
			Lang:           "en_US.UTF-8",
			KeyboardLayout: "de",
			UpdateSystem:   true,
			ChronyServers:  []string{"pool.ntp.org"},
			SSHKeys: map[string][]string{
				"root": {"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITESTKEY test@proxctl"},
			},
			Packages: &config.PackagesConfig{
				Base: []string{"htop"},
				Post: []string{"tmux"},
			},
			Firewall: &config.KSFirewall{Enabled: true},
			AdditionalUsers: []config.AdditionalUser{
				{Name: "deploy", Wheel: true, SSHKey: "ssh-rsa AAAAB3DEPLOYKEY deploy@proxctl"},
			},
			Sudo: &config.SudoConfig{WheelNopasswd: true},
		},
	}

	env := &config.Env{
		Version: "1",
		Kind:    "Env",
		Metadata: config.EnvMetadata{
			Name:   "testenv",
			Domain: "example.com",
		},
	}
	env.Spec.Hypervisor.Inline = hyp
	return env
}

func TestRendererOL9(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, err := r.Render(buildEnv(), "web01")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	mustContain := []string{
		"#version=OL9",
		"web01.example.com",
		"Europe/Berlin",
		"10.10.0.100",
		"255.255.255.0",
		"10.10.0.1",
		"AAAAITESTKEY",
		"user --name=deploy",
		"AAAAB3DEPLOYKEY",
		"firewall --enabled --ssh",
		"pool.ntp.org",
		"NOPASSWD",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("rendered kickstart missing %q\n---\n%s", s, out)
		}
	}
}

func TestSupportedDistros(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	ds := r.SupportedDistros()
	wantAny := map[string]bool{"oraclelinux8": false, "oraclelinux9": false, "ubuntu2204": false}
	for _, d := range ds {
		if _, ok := wantAny[d]; ok {
			wantAny[d] = true
		}
	}
	for d, found := range wantAny {
		if !found {
			t.Errorf("SupportedDistros missing %q (got %v)", d, ds)
		}
	}
}

func TestRenderUbuntu(t *testing.T) {
	env := buildEnv()
	env.Spec.Hypervisor.Inline.Kickstart.Distro = "ubuntu2204"
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, err := r.Render(env, "web01")
	if err != nil {
		t.Fatalf("Render ubuntu: %v", err)
	}
	if !strings.Contains(out, "d-i netcfg/get_hostname string web01") {
		t.Errorf("ubuntu preseed missing hostname directive: %s", out)
	}
}

func TestCidrHelpers(t *testing.T) {
	if got := cidrIP("10.0.0.5/24"); got != "10.0.0.5" {
		t.Errorf("cidrIP: got %q", got)
	}
	if got := cidrNetmask("10.0.0.5/24"); got != "255.255.255.0" {
		t.Errorf("cidrNetmask: got %q", got)
	}
	if got := cidrPrefix("10.0.0.5/16"); got != "16" {
		t.Errorf("cidrPrefix: got %q", got)
	}
}

func TestRenderUnknownNode(t *testing.T) {
	r, _ := NewRenderer()
	if _, err := r.Render(buildEnv(), "nope"); err == nil {
		t.Errorf("expected error for unknown node")
	}
}

func TestRenderOL8(t *testing.T) {
	env := buildEnv()
	env.Spec.Hypervisor.Inline.Kickstart.Distro = "oraclelinux8"
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, err := r.Render(env, "web01")
	if err != nil {
		t.Fatalf("Render OL8: %v", err)
	}
	for _, s := range []string{
		"#version=OL8",
		"web01.example.com",
		"Europe/Berlin",
		"10.10.0.100",
		"255.255.255.0",
		"AAAAITESTKEY",
		"user --name=deploy",
		"firewall --enabled --ssh",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("OL8 kickstart missing %q", s)
		}
	}
}

func TestRenderErrorPaths(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	if _, err := r.Render(nil, "web01"); err == nil {
		t.Error("expected error for nil env")
	}

	// env with no hypervisor resolved
	bad := &config.Env{Version: "1", Kind: "Env"}
	if _, err := r.Render(bad, "web01"); err == nil {
		t.Error("expected error for unresolved hypervisor")
	}

	// no kickstart config
	env := buildEnv()
	env.Spec.Hypervisor.Inline.Kickstart = nil
	if _, err := r.Render(env, "web01"); err == nil {
		t.Error("expected error for missing kickstart")
	}

	// empty distro
	env2 := buildEnv()
	env2.Spec.Hypervisor.Inline.Kickstart.Distro = ""
	if _, err := r.Render(env2, "web01"); err == nil {
		t.Error("expected error for empty distro")
	}

	// unsupported distro
	env3 := buildEnv()
	env3.Spec.Hypervisor.Inline.Kickstart.Distro = "nope"
	if _, err := r.Render(env3, "web01"); err == nil {
		t.Error("expected error for unsupported distro")
	}
}

func TestPickEntryNone(t *testing.T) {
	// Template with no recognised entry name → pickEntry returns "".
	tmpl := template.New("x")
	template.Must(tmpl.New("other.tmpl").Parse("nope"))
	if got := pickEntry(tmpl); got != "" {
		t.Errorf("pickEntry: want empty got %q", got)
	}
}

func TestCidrHelpersInvalid(t *testing.T) {
	// Invalid CIDR → fallback to bare IP
	if got := cidrIP("10.0.0.5"); got != "10.0.0.5" {
		t.Errorf("cidrIP bare: got %q", got)
	}
	// Truly invalid → return s
	if got := cidrIP("garbage"); got != "garbage" {
		t.Errorf("cidrIP garbage: got %q", got)
	}
	if got := cidrNetmask("not-a-cidr"); got != "" {
		t.Errorf("cidrNetmask invalid: got %q", got)
	}
	if got := cidrPrefix("not-a-cidr"); got != "" {
		t.Errorf("cidrPrefix invalid: got %q", got)
	}
	// IPv6 netmask → "/NN"
	if got := cidrNetmask("2001:db8::/64"); got != "/64" {
		t.Errorf("cidrNetmask ipv6: got %q", got)
	}
}

func TestJoinLines(t *testing.T) {
	got := joinLines([]string{"a\n", "", "b", "c\n"})
	want := "a\nb\nc"
	if got != want {
		t.Errorf("joinLines: got %q want %q", got, want)
	}
	if got := joinLines(nil); got != "" {
		t.Errorf("joinLines nil: got %q", got)
	}
}

func TestRenderWithNetworksAndCluster(t *testing.T) {
	env := buildEnv()
	env.Spec.Networks.Inline = &config.Networks{
		Kind: "Networks",
		Zones: map[string]config.NetworkZone{
			"public": {},
		},
	}
	env.Spec.Cluster = &config.Ref[config.Cluster]{
		Inline: &config.Cluster{
			Kind:    "Cluster",
			ScanIPs: []string{"10.0.0.200", "10.0.0.201"},
		},
	}
	r, _ := NewRenderer()
	if _, err := r.Render(env, "web01"); err != nil {
		t.Fatalf("Render with networks+cluster: %v", err)
	}
}

func TestNewRendererFromFS_MissingTemplatesDir(t *testing.T) {
	if _, err := newRendererFromFS(fstest.MapFS{}); err == nil {
		t.Error("expected error when templates/ dir missing")
	}
}

func TestNewRendererFromFS_EmptyDistroDirSkipped(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/common/x.tmpl":           {Data: []byte("{{ define \"x\" }}x{{ end }}")},
		"templates/emptydistro/.keep":       {Data: []byte("")},
		"templates/real/base.ks.tmpl":       {Data: []byte("hello")},
	}
	r, err := newRendererFromFS(fsys)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	for _, d := range r.SupportedDistros() {
		if d == "emptydistro" {
			t.Errorf("emptydistro should have been skipped")
		}
	}
}

func TestNewRendererFromFS_BadCommonTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/common/bad.tmpl":     {Data: []byte("{{ .Unterminated ")},
		"templates/distro/base.ks.tmpl": {Data: []byte("hello")},
	}
	if _, err := newRendererFromFS(fsys); err == nil {
		t.Error("expected parse common error")
	}
}

func TestNewRendererFromFS_BadDistroTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/distro/base.ks.tmpl": {Data: []byte("{{ .Unterminated ")},
	}
	if _, err := newRendererFromFS(fsys); err == nil {
		t.Error("expected parse distro error")
	}
}

func TestRender_ExecuteTemplateFailure(t *testing.T) {
	// Template references a method that will fail when executed with our context.
	fsys := fstest.MapFS{
		"templates/distro/base.ks.tmpl": {Data: []byte("{{ .Env.Does.Not.Exist }}")},
	}
	r, err := newRendererFromFS(fsys)
	if err != nil {
		t.Fatalf("loader: %v", err)
	}
	env := buildEnv()
	env.Spec.Hypervisor.Inline.Kickstart.Distro = "distro"
	if _, err := r.Render(env, "web01"); err == nil {
		t.Error("expected execute failure")
	}
}

func TestRender_NoEntryTemplate(t *testing.T) {
	// Distro templates exist but none match the known entry-point names.
	fsys := fstest.MapFS{
		"templates/distro/other.tmpl": {Data: []byte("nope")},
	}
	r, err := newRendererFromFS(fsys)
	if err != nil {
		t.Fatalf("loader: %v", err)
	}
	env := buildEnv()
	env.Spec.Hypervisor.Inline.Kickstart.Distro = "distro"
	if _, err := r.Render(env, "web01"); err == nil {
		t.Error("expected no-entry-template error")
	}
}

func TestRenderNoDomain(t *testing.T) {
	env := buildEnv()
	env.Metadata.Domain = ""
	r, _ := NewRenderer()
	out, err := r.Render(env, "web01")
	if err != nil {
		t.Fatalf("Render no domain: %v", err)
	}
	// FQDN should be hostname-only.
	if !strings.Contains(out, "for web01\n") && !strings.Contains(out, "for web01\r") {
		// accept the line "# proxctl-generated kickstart for web01"
		if !strings.Contains(out, "for web01") {
			t.Errorf("expected FQDN==hostname without domain, got output without 'for web01'")
		}
	}
}
