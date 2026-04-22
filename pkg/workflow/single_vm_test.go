package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itunified-io/proxctl/pkg/config"
	"github.com/itunified-io/proxctl/pkg/kickstart"
	"github.com/itunified-io/proxctl/pkg/proxmox"
)

func testEnv() *config.Env {
	hyp := &config.Hypervisor{
		Kind: "Hypervisor",
		Nodes: map[string]config.Node{
			"web01": {
				Proxmox: config.ProxmoxRef{NodeName: "pve01", VMID: 201},
				IPs:     map[string]string{"public": "10.0.0.10"},
				Resources: &config.Resources{
					Memory: 2048, Cores: 2, Sockets: 1, BIOS: "seabios",
				},
				NICs: []config.NIC{
					{NameField: "eth0", Bridge: "vmbr0", Usage: "public", Bootproto: "dhcp"},
				},
				Disks: []config.Disk{
					{ID: 0, Size: "16G", Storage: "local-lvm", Interface: "scsi0"},
				},
			},
		},
		Kickstart: &config.KickstartConfig{
			Distro: "oraclelinux9",
		},
		ISO: &config.ISOConfig{
			Storage:          "local",
			Image:            "OL9.iso",
			KickstartStorage: "local",
		},
	}
	env := &config.Env{
		Version: "1", Kind: "Env",
		Metadata: config.EnvMetadata{Name: "t", Domain: "example.com"},
	}
	env.Spec.Hypervisor.Inline = hyp
	return env
}

// mockPVE is a minimal Proxmox stub that returns reasonable responses for the
// endpoints the workflow hits during Plan + Apply.
func mockPVE(t *testing.T, recorder *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*recorder = append(*recorder, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			// VM doesn't exist → 500 with "does not exist"
			http.Error(w, `{"data":null,"errors":{"vmid":"does not exist"}}`, http.StatusInternalServerError)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/qemu"):
			// CreateVM — return a fake UPID (empty string won't trigger WaitForTask)
			w.Write([]byte(`{"data":""}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/status/start"):
			w.Write([]byte(`{"data":""}`))
		case strings.HasSuffix(r.URL.Path, "/upload"):
			w.Write([]byte(`{"data":""}`))
		default:
			w.Write([]byte(`{"data":null}`))
		}
	}))
}

func TestPlan(t *testing.T) {
	calls := []string{}
	srv := mockPVE(t, &calls)
	defer srv.Close()
	client, err := proxmox.NewClient(proxmox.ClientOpts{
		Endpoint: srv.URL, TokenID: "t@pve!k", TokenSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	w := &SingleVMWorkflow{
		Config:   testEnv(),
		NodeName: "web01",
		Client:   client,
	}
	changes, err := w.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	wantKinds := []string{"render-kickstart", "build-iso", "upload-iso", "create-vm", "start-vm"}
	if len(changes) != len(wantKinds) {
		t.Fatalf("want %d changes got %d: %+v", len(wantKinds), len(changes), changes)
	}
	for i, k := range wantKinds {
		if changes[i].Kind != k {
			t.Errorf("change[%d] kind: want %q got %q", i, k, changes[i].Kind)
		}
	}
}

func TestPlanDryRunApply(t *testing.T) {
	calls := []string{}
	srv := mockPVE(t, &calls)
	defer srv.Close()
	client, err := proxmox.NewClient(proxmox.ClientOpts{
		Endpoint: srv.URL, TokenID: "t@pve!k", TokenSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	rnd, err := kickstart.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	w := &SingleVMWorkflow{
		Config:   testEnv(),
		NodeName: "web01",
		Client:   client,
		Renderer: rnd,
		DryRun:   true,
	}
	changes, err := w.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Dry-run apply should not hit the server for create/start.
	if err := w.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
}

func TestBuildCreateOpts(t *testing.T) {
	env := testEnv()
	hyp := env.Spec.Hypervisor.Resolved()
	node := hyp.Nodes["web01"]
	r := &resolved{hyp: hyp, node: node, ks: hyp.Kickstart, iso: hyp.ISO}
	opts := buildCreateOpts(env, "web01", &node, r)
	if opts.Node != "pve01" || opts.VMID != 201 {
		t.Errorf("node/vmid wrong: %+v", opts)
	}
	if len(opts.Disks) != 1 || opts.Disks[0].Interface != "scsi0" {
		t.Errorf("disks wrong: %+v", opts.Disks)
	}
	if len(opts.NICs) != 1 || opts.NICs[0].Bridge != "vmbr0" {
		t.Errorf("nics wrong: %+v", opts.NICs)
	}
	if opts.ISOFile != "local:iso/OL9.iso" {
		t.Errorf("ISOFile wrong: %q", opts.ISOFile)
	}
}

func TestResolveMissingNode(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "nope"}
	if _, err := w.resolve(); err == nil {
		t.Errorf("expected error for missing node")
	}
}

func TestDownNotExist(t *testing.T) {
	calls := []string{}
	srv := mockPVE(t, &calls)
	defer srv.Close()
	client, _ := proxmox.NewClient(proxmox.ClientOpts{
		Endpoint: srv.URL, TokenID: "t@pve!k", TokenSecret: "secret",
	})
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01", Client: client,
	}
	if err := w.Down(context.Background(), false); err != nil {
		t.Errorf("Down on absent VM: %v", err)
	}
}

func TestVerifyMissing(t *testing.T) {
	calls := []string{}
	srv := mockPVE(t, &calls)
	defer srv.Close()
	client, _ := proxmox.NewClient(proxmox.ClientOpts{
		Endpoint: srv.URL, TokenID: "t@pve!k", TokenSecret: "secret",
	})
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01", Client: client,
	}
	if err := w.Verify(context.Background()); err == nil {
		t.Errorf("expected verify to fail when VM does not exist")
	}
}

func TestRollbackAbsent(t *testing.T) {
	calls := []string{}
	srv := mockPVE(t, &calls)
	defer srv.Close()
	client, _ := proxmox.NewClient(proxmox.ClientOpts{
		Endpoint: srv.URL, TokenID: "t@pve!k", TokenSecret: "secret",
	})
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01", Client: client,
	}
	if err := w.Rollback(context.Background(), nil); err != nil {
		t.Errorf("Rollback on absent VM: %v", err)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "a", "b") != "a" {
		t.Errorf("firstNonEmpty wrong")
	}
	if firstNonEmpty("", "") != "" {
		t.Errorf("firstNonEmpty empty wrong")
	}
}
