package workflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

func TestPlan_SkipKickstartBuild(t *testing.T) {
	// With SkipKickstartBuild=true, the plan should drop render/build/upload
	// and instead include a verify-kickstart-iso step, then create-vm + start-vm.
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
		Config:             testEnv(),
		NodeName:           "web01",
		Client:             client,
		SkipKickstartBuild: true,
	}
	changes, err := w.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	wantKinds := []string{"verify-kickstart-iso", "create-vm", "start-vm"}
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

// --- broader coverage tests -----------------------------------------------

// pveState is a stateful mock of the Proxmox HTTP API, sufficient to drive a
// full Plan → Apply → Verify → Rollback cycle.
type pveState struct {
	mu sync.Mutex

	vmExists  bool
	vmRunning bool

	// Failure injection: path suffix → HTTP status code.
	failOn map[string]int
}

func newPVE() *pveState {
	return &pveState{failOn: map[string]int{}}
}

func (p *pveState) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()

		for suffix, code := range p.failOn {
			if strings.Contains(r.URL.Path, suffix) {
				http.Error(w, `{"errors":{"simulated":"forced"}}`, code)
				return
			}
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			if !p.vmExists {
				http.Error(w, `{"data":null,"errors":{"vmid":"does not exist"}}`, http.StatusInternalServerError)
				return
			}
			status := "stopped"
			if p.vmRunning {
				status = "running"
			}
			fmt.Fprintf(w, `{"data":{"vmid":"201","name":"web01","status":%q,"cpus":2,"maxmem":2147483648}}`, status)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/qemu"):
			p.vmExists = true
			w.Write([]byte(`{"data":""}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/status/start"):
			p.vmRunning = true
			w.Write([]byte(`{"data":""}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/status/stop"),
			r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/status/shutdown"):
			p.vmRunning = false
			w.Write([]byte(`{"data":""}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/qemu/"):
			p.vmExists = false
			p.vmRunning = false
			w.Write([]byte(`{"data":""}`))
		case strings.HasSuffix(r.URL.Path, "/upload"):
			w.Write([]byte(`{"data":""}`))
		default:
			w.Write([]byte(`{"data":null}`))
		}
	}))
}

func (p *pveState) client(t *testing.T, srv *httptest.Server) *proxmox.Client {
	t.Helper()
	c, err := proxmox.NewClient(proxmox.ClientOpts{
		Endpoint: srv.URL, TokenID: "t@pve!k", TokenSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// stubBuilder is an ISOBuilder substitute that writes a marker file without
// invoking xorriso. We shell out to the real ISOBuilder for no-tool tests and
// use this for happy-path coverage.
func stubISOBuilder(t *testing.T) *kickstart.ISOBuilder {
	t.Helper()
	dir := t.TempDir()
	fakeTool := dir + "/xorriso"
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ \"$1\" = \"-o\" ]; then echo stub > \"$2\"; fi; shift; done\nexit 0\n"
	if err := os.WriteFile(fakeTool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", orig) })

	bootDir := t.TempDir()
	if err := os.WriteFile(bootDir+"/isolinux.bin", []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := kickstart.NewISOBuilder(bootDir)
	b.WorkDir = t.TempDir()
	return b
}

func TestApply_FullHappyPath(t *testing.T) {
	state := newPVE()
	srv := state.server(t)
	defer srv.Close()
	client := state.client(t, srv)

	rnd, err := kickstart.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	w := &SingleVMWorkflow{
		Config:   testEnv(),
		NodeName: "web01",
		Client:   client,
		Renderer: rnd,
		Builder:  stubISOBuilder(t),
	}
	changes, err := w.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if err := w.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := w.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestApply_RendererNotSet(t *testing.T) {
	state := newPVE()
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	err := w.Apply(context.Background(), []Change{{Kind: "render-kickstart"}})
	if err == nil || !strings.Contains(err.Error(), "Renderer") {
		t.Errorf("want renderer error, got %v", err)
	}
}

func TestApply_RenderFailure(t *testing.T) {
	state := newPVE()
	srv := state.server(t)
	defer srv.Close()
	env := testEnv()
	env.Spec.Hypervisor.Inline.Kickstart.Distro = "nope"
	rnd, _ := kickstart.NewRenderer()
	w := &SingleVMWorkflow{Config: env, NodeName: "web01", Client: state.client(t, srv), Renderer: rnd}
	err := w.Apply(context.Background(), []Change{{Kind: "render-kickstart"}})
	if err == nil || !strings.Contains(err.Error(), "render") {
		t.Errorf("want render error, got %v", err)
	}
}

func TestApply_BuilderNotSet(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	err := w.Apply(context.Background(), []Change{{Kind: "build-iso"}})
	if err == nil || !strings.Contains(err.Error(), "Builder") {
		t.Errorf("want builder error, got %v", err)
	}
}

func TestApply_BuildISOFailure(t *testing.T) {
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01",
		Builder: &kickstart.ISOBuilder{}, // no Tool, no BootloaderDir → error
	}
	err := w.Apply(context.Background(), []Change{{Kind: "build-iso"}})
	if err == nil || !strings.Contains(err.Error(), "build-iso") {
		t.Errorf("want build-iso error, got %v", err)
	}
}

func TestApply_UploadISO_MissingClient(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	err := w.Apply(context.Background(), []Change{{Kind: "upload-iso"}})
	if err == nil || !strings.Contains(err.Error(), "Client") {
		t.Errorf("want client error, got %v", err)
	}
}

func TestApply_UploadISO_NoStorage(t *testing.T) {
	state := newPVE()
	srv := state.server(t)
	defer srv.Close()
	env := testEnv()
	env.Spec.Hypervisor.Inline.ISO.KickstartStorage = ""
	w := &SingleVMWorkflow{Config: env, NodeName: "web01", Client: state.client(t, srv)}
	err := w.Apply(context.Background(), []Change{{Kind: "upload-iso"}})
	if err == nil || !strings.Contains(err.Error(), "kickstart_storage") {
		t.Errorf("want kickstart_storage error, got %v", err)
	}
}

func TestApply_UploadISO_EmptyIsoPath(t *testing.T) {
	state := newPVE()
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	err := w.Apply(context.Background(), []Change{{Kind: "upload-iso"}})
	if err == nil || !strings.Contains(err.Error(), "iso path") {
		t.Errorf("want iso path error, got %v", err)
	}
}

func TestApply_UploadISO_Failure(t *testing.T) {
	state := newPVE()
	state.failOn["/upload"] = http.StatusInternalServerError
	srv := state.server(t)
	defer srv.Close()
	rnd, _ := kickstart.NewRenderer()
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01", Client: state.client(t, srv),
		Renderer: rnd, Builder: stubISOBuilder(t),
	}
	err := w.Apply(context.Background(), []Change{
		{Kind: "render-kickstart"}, {Kind: "build-iso"}, {Kind: "upload-iso"},
	})
	if err == nil || !strings.Contains(err.Error(), "upload-iso") {
		t.Errorf("want upload-iso error, got %v", err)
	}
}

func TestApply_CreateVM_MissingClient(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	err := w.Apply(context.Background(), []Change{{Kind: "create-vm"}})
	if err == nil || !strings.Contains(err.Error(), "Client") {
		t.Errorf("want client error, got %v", err)
	}
}

func TestApply_CreateVM_Failure(t *testing.T) {
	state := newPVE()
	state.failOn["/qemu"] = http.StatusInternalServerError
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	err := w.Apply(context.Background(), []Change{{Kind: "create-vm"}})
	if err == nil || !strings.Contains(err.Error(), "create-vm") {
		t.Errorf("want create-vm error, got %v", err)
	}
}

func TestApply_StartVM_MissingClient(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	err := w.Apply(context.Background(), []Change{{Kind: "start-vm"}})
	if err == nil || !strings.Contains(err.Error(), "Client") {
		t.Errorf("want client error, got %v", err)
	}
}

func TestApply_StartVM_Failure(t *testing.T) {
	state := newPVE()
	state.failOn["/status/start"] = http.StatusInternalServerError
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	err := w.Apply(context.Background(), []Change{{Kind: "start-vm"}})
	if err == nil || !strings.Contains(err.Error(), "start-vm") {
		t.Errorf("want start-vm error, got %v", err)
	}
}

func TestApply_UnknownKind(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	err := w.Apply(context.Background(), []Change{{Kind: "wat"}})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("want unknown-kind error, got %v", err)
	}
}

func TestApply_ResolveFailure(t *testing.T) {
	w := &SingleVMWorkflow{Config: nil, NodeName: "web01"}
	if err := w.Apply(context.Background(), nil); err == nil {
		t.Error("want resolve error")
	}
}

func TestVerify_NotRunning(t *testing.T) {
	state := newPVE()
	state.vmExists = true
	state.vmRunning = false
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if err := w.Verify(context.Background()); err == nil {
		t.Error("want verify to fail when VM not running")
	}
}

func TestVerify_Running(t *testing.T) {
	state := newPVE()
	state.vmExists = true
	state.vmRunning = true
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if err := w.Verify(context.Background()); err != nil {
		t.Errorf("verify running: %v", err)
	}
}

func TestVerify_ResolveFailure(t *testing.T) {
	w := &SingleVMWorkflow{Config: nil, NodeName: "web01"}
	if err := w.Verify(context.Background()); err == nil {
		t.Error("want resolve error")
	}
}

func TestVerify_NoClient(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	if err := w.Verify(context.Background()); err == nil {
		t.Error("want client error")
	}
}

func TestRollback_ExistingVM(t *testing.T) {
	state := newPVE()
	state.vmExists = true
	state.vmRunning = true
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if err := w.Rollback(context.Background(), nil); err != nil {
		t.Errorf("Rollback: %v", err)
	}
	if state.vmExists {
		t.Error("VM should be gone after rollback")
	}
}

func TestRollback_ResolveFailure(t *testing.T) {
	w := &SingleVMWorkflow{Config: nil, NodeName: "web01"}
	if err := w.Rollback(context.Background(), nil); err == nil {
		t.Error("want resolve error")
	}
}

func TestRollback_NoClient(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	if err := w.Rollback(context.Background(), nil); err == nil {
		t.Error("want client error")
	}
}

func TestRollback_Idempotent(t *testing.T) {
	state := newPVE()
	state.vmExists = true
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if err := w.Rollback(context.Background(), nil); err != nil {
		t.Fatalf("Rollback 1: %v", err)
	}
	if err := w.Rollback(context.Background(), nil); err != nil {
		t.Errorf("Rollback 2 (idempotent): %v", err)
	}
}

func TestDown_ExistingVM(t *testing.T) {
	state := newPVE()
	state.vmExists = true
	state.vmRunning = true
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if err := w.Down(context.Background(), true); err != nil {
		t.Errorf("Down: %v", err)
	}
}

func TestDown_NoClient(t *testing.T) {
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01"}
	if err := w.Down(context.Background(), false); err == nil {
		t.Error("want client error")
	}
}

func TestDown_ResolveFailure(t *testing.T) {
	w := &SingleVMWorkflow{Config: nil, NodeName: "web01"}
	if err := w.Down(context.Background(), false); err == nil {
		t.Error("want resolve error")
	}
}

func TestUp(t *testing.T) {
	state := newPVE()
	srv := state.server(t)
	defer srv.Close()
	rnd, _ := kickstart.NewRenderer()
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01", Client: state.client(t, srv),
		Renderer: rnd, Builder: stubISOBuilder(t),
	}
	if err := w.Up(context.Background()); err != nil {
		t.Errorf("Up: %v", err)
	}
}

func TestUp_PlanFailure(t *testing.T) {
	// VM already exists → Plan aborts.
	state := newPVE()
	state.vmExists = true
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if err := w.Up(context.Background()); err == nil {
		t.Error("want plan failure to propagate")
	}
}

func TestUp_ApplyFailure(t *testing.T) {
	state := newPVE()
	state.failOn["/qemu"] = http.StatusInternalServerError
	srv := state.server(t)
	defer srv.Close()
	rnd, _ := kickstart.NewRenderer()
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01", Client: state.client(t, srv),
		Renderer: rnd, Builder: stubISOBuilder(t),
	}
	if err := w.Up(context.Background()); err == nil {
		t.Error("want apply failure to propagate")
	}
}

func TestUp_VerifyWarns(t *testing.T) {
	// VM creates successfully but start-vm never happens because /start fails.
	// Actually Up returns nil on Apply success + Verify warning only, so we
	// want a state where Apply succeeds but Verify finds VM non-running.
	// Configure server to accept create/start but GET returns stopped.
	state := newPVE()
	// Don't set vmRunning in server's start handler: override via failOn nope.
	// Actually server sets vmRunning=true on /start. We want the opposite:
	// override status/current to return "stopped" always.
	// Simulate by keeping state default but manually setting running=false
	// after start. We can't intercept; easier: Apply succeeds, verify gets "stopped".
	// Since pveState.vmRunning flips true on /start, we need a different
	// server. Use a variant: dedicated handler.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			if !state.vmExists {
				http.Error(w, `{"errors":{"vmid":"does not exist"}}`, 500)
				return
			}
			fmt.Fprintf(w, `{"data":{"vmid":"201","status":"stopped"}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/qemu"):
			state.vmExists = true
			w.Write([]byte(`{"data":""}`))
		default:
			w.Write([]byte(`{"data":""}`))
		}
	}))
	defer srv.Close()
	rnd, _ := kickstart.NewRenderer()
	w := &SingleVMWorkflow{
		Config: testEnv(), NodeName: "web01",
		Client: func() *proxmox.Client {
			c, _ := proxmox.NewClient(proxmox.ClientOpts{Endpoint: srv.URL, TokenID: "t@pve!k", TokenSecret: "s"})
			return c
		}(),
		Renderer: rnd, Builder: stubISOBuilder(t),
	}
	// Up should return nil despite verify warning.
	if err := w.Up(context.Background()); err != nil {
		t.Errorf("Up returned error when only verify should warn: %v", err)
	}
}

func TestBuildCreateOpts_OVMF_StorageClass(t *testing.T) {
	env := testEnv()
	node := env.Spec.Hypervisor.Inline.Nodes["web01"]
	node.Resources.BIOS = "ovmf"
	// Disk uses StorageClass fallback (no direct Storage set).
	node.Disks[0].Storage = ""
	node.Disks[0].StorageClass = "fast-ssd"
	env.Spec.Hypervisor.Inline.Nodes["web01"] = node
	hyp := env.Spec.Hypervisor.Resolved()
	n := hyp.Nodes["web01"]
	r := &resolved{hyp: hyp, node: n, ks: hyp.Kickstart, iso: hyp.ISO}
	opts := buildCreateOpts(env, "web01", &n, r)
	if opts.EFIDisk == nil {
		t.Errorf("expected EFIDisk for ovmf")
	}
	if opts.Disks[0].Storage != "fast-ssd" {
		t.Errorf("want fast-ssd got %q", opts.Disks[0].Storage)
	}
}

func TestBuildCreateOpts_NoResources(t *testing.T) {
	env := testEnv()
	node := env.Spec.Hypervisor.Inline.Nodes["web01"]
	node.Resources = nil
	node.Disks[0].Interface = "" // → default "scsi0"
	node.NICs[0].Bridge = ""     // → default "vmbr0"
	env.Spec.Hypervisor.Inline.Nodes["web01"] = node
	hyp := env.Spec.Hypervisor.Resolved()
	n := hyp.Nodes["web01"]
	r := &resolved{hyp: hyp, node: n, ks: hyp.Kickstart, iso: hyp.ISO}
	opts := buildCreateOpts(env, "web01", &n, r)
	if opts.Disks[0].Interface != "scsi0" {
		t.Errorf("want scsi0 got %q", opts.Disks[0].Interface)
	}
	if opts.NICs[0].Bridge != "vmbr0" {
		t.Errorf("want vmbr0 got %q", opts.NICs[0].Bridge)
	}
}

func TestStorageForDisk(t *testing.T) {
	if got := storageForDisk([]config.Disk{{Size: "10G"}, {Size: "20G", Storage: "x"}}); got != "x" {
		t.Errorf("want x got %q", got)
	}
	if got := storageForDisk(nil); got != "" {
		t.Errorf("want empty got %q", got)
	}
}

func TestSafeHelpers_Nil(t *testing.T) {
	if safeDistro(nil) != "(no-kickstart)" {
		t.Error("safeDistro nil")
	}
	if safeMemory(nil) != 0 {
		t.Error("safeMemory nil")
	}
	if safeCores(nil) != 0 {
		t.Error("safeCores nil")
	}
}

func TestPlan_VMAlreadyExists(t *testing.T) {
	state := newPVE()
	state.vmExists = true
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if _, err := w.Plan(context.Background()); err == nil {
		t.Error("want plan error when VM exists")
	}
}

func TestPlan_ResolveFailure(t *testing.T) {
	w := &SingleVMWorkflow{Config: nil, NodeName: "web01"}
	if _, err := w.Plan(context.Background()); err == nil {
		t.Error("want resolve error")
	}
}

func TestPlan_VMExistsCheck_TransportError(t *testing.T) {
	// Server returns 502 — not interpreted as "does not exist".
	state := newPVE()
	state.failOn["/status/current"] = http.StatusBadGateway
	srv := state.server(t)
	defer srv.Close()
	w := &SingleVMWorkflow{Config: testEnv(), NodeName: "web01", Client: state.client(t, srv)}
	if _, err := w.Plan(context.Background()); err == nil {
		t.Error("want vm-exists check error")
	}
}

func TestPlan_NoKickstartStorage(t *testing.T) {
	// When iso.kickstart_storage is empty, no upload-iso change is emitted.
	state := newPVE()
	srv := state.server(t)
	defer srv.Close()
	env := testEnv()
	env.Spec.Hypervisor.Inline.ISO.KickstartStorage = ""
	w := &SingleVMWorkflow{Config: env, NodeName: "web01", Client: state.client(t, srv)}
	changes, err := w.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, c := range changes {
		if c.Kind == "upload-iso" {
			t.Errorf("did not expect upload-iso change: %+v", changes)
		}
	}
}

func TestResolve_NilConfig(t *testing.T) {
	w := &SingleVMWorkflow{Config: nil, NodeName: "web01"}
	if _, err := w.resolve(); err == nil {
		t.Error("want nil-config error")
	}
}

func TestResolve_UnresolvedHypervisor(t *testing.T) {
	env := &config.Env{Version: "1", Kind: "Env"}
	w := &SingleVMWorkflow{Config: env, NodeName: "web01"}
	if _, err := w.resolve(); err == nil {
		t.Error("want unresolved hypervisor error")
	}
}

func TestResolve_ClusterResolved(t *testing.T) {
	env := testEnv()
	env.Spec.Cluster = &config.Ref[config.Cluster]{
		Inline: &config.Cluster{Kind: "Cluster"},
	}
	w := &SingleVMWorkflow{Config: env, NodeName: "web01"}
	r, err := w.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.cluster == nil {
		t.Error("want cluster resolved")
	}
}
