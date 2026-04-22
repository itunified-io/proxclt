package kickstart

import (
	"strings"
	"testing"

	"github.com/itunified-io/proxclt/pkg/config"
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
				"root": {"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITESTKEY test@proxclt"},
			},
			Packages: &config.PackagesConfig{
				Base: []string{"htop"},
				Post: []string{"tmux"},
			},
			Firewall: &config.KSFirewall{Enabled: true},
			AdditionalUsers: []config.AdditionalUser{
				{Name: "deploy", Wheel: true, SSHKey: "ssh-rsa AAAAB3DEPLOYKEY deploy@proxclt"},
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
