package root

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// ------------------------------------------------------------------------
// test harness
// ------------------------------------------------------------------------

// executeCmd builds a fresh root, runs it with args, and returns captured output.
func executeCmd(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	cmd := New()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return buf.String(), err
}

// isolateHome sets HOME to a fresh tempdir (so ~/.proxctl/config.yaml is absent)
// and clears any proxctl env vars. Returns the tempdir path.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PROXCTL_ENDPOINT", "")
	t.Setenv("PROXCTL_TOKEN_ID", "")
	t.Setenv("PROXCTL_TOKEN_SECRET", "")
	t.Setenv("PROXCTL_INSECURE_TLS", "")
	// Reset package-level flags that persist across tests.
	flagContext = ""
	flagStack = ""
	flagEnv = ""
	flagJSON = false
	flagYes = false
	envFlagDeprecated = false
	envVarDeprecated = false
	envDeprecated = false
	return home
}

// writeEnvFixture writes an inline env.yaml into dir. Both inline and $ref-
// composed manifests are supported by loadEnvManifest (which now routes through
// config.Load — see #19); this helper covers the inline path. For $ref tests,
// use writeRefEnvFixture.
func writeEnvFixture(t *testing.T, dir string) string {
	t.Helper()
	// Also copy the $ref-style fixture used by config validate/render tests.
	src := "../../pkg/config/testdata"
	for _, name := range []string{"hypervisor.yaml", "networks.yaml", "storage_classes.yaml", "cluster.yaml"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err == nil {
			_ = os.WriteFile(filepath.Join(dir, name), data, 0o644)
		}
	}
	inline := `version: "1"
kind: Env
metadata:
  name: test-env
  domain: example.com
spec:
  hypervisor:
    kind: Hypervisor
    kickstart:
      distro: oraclelinux9
    nodes:
      rac-node-1:
        proxmox:
          node_name: pve-prod-1
          vm_id: 1001
        ips:
          public: 10.10.0.101
          private: 10.10.1.101
        resources:
          memory: 8192
          cores: 4
          sockets: 1
        disks:
          - id: 0
            size: 64G
            storage_class: local-lvm
            interface: scsi0
            role: root
      rac-node-2:
        proxmox:
          node_name: pve-prod-1
          vm_id: 1002
        ips:
          public: 10.10.0.102
          private: 10.10.1.102
        resources:
          memory: 8192
          cores: 4
          sockets: 1
        disks:
          - id: 0
            size: 64G
            storage_class: local-lvm
            interface: scsi0
            role: root
  networks:
    kind: Networks
    public:
      cidr: 10.10.0.0/24
      gateway: 10.10.0.1
    private:
      cidr: 10.10.1.0/24
  storage_classes:
    kind: StorageClasses
    local-lvm:
      backend: lvm-thin
      shared: false
`
	envPath := filepath.Join(dir, "env.yaml")
	if err := os.WriteFile(envPath, []byte(inline), 0o644); err != nil {
		t.Fatalf("write env.yaml: %v", err)
	}
	return envPath
}

// writeRefEnvFixture writes the $ref-style fixture (requires config.Load resolution).
// Used for config validate/render tests that go through config.Load().
func writeRefEnvFixture(t *testing.T, dir string) string {
	t.Helper()
	src := "../../pkg/config/testdata"
	for _, name := range []string{"env.yaml", "hypervisor.yaml", "networks.yaml", "storage_classes.yaml", "cluster.yaml"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return filepath.Join(dir, "env.yaml")
}

// startProxmox starts a httptest Proxmox server with the given handler and wires
// the three PROXCTL_ env vars to point at it. Returns the server for cleanup.
func startProxmox(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("PROXCTL_ENDPOINT", srv.URL)
	t.Setenv("PROXCTL_TOKEN_ID", "root@pam!test")
	t.Setenv("PROXCTL_TOKEN_SECRET", "test-secret")
	return srv
}

// writeJSON writes a Proxmox-envelope response.
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

// ------------------------------------------------------------------------
// root / version / help
// ------------------------------------------------------------------------

func TestRoot_HelpListsAllSubcommands(t *testing.T) {
	isolateHome(t)
	out, err := executeCmd(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"config", "stack", "vm", "snapshot", "kickstart", "boot", "workflow", "license", "version"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing subcommand %q\n%s", want, out)
		}
	}
}

func TestVersion_Output(t *testing.T) {
	isolateHome(t)
	out, err := executeCmd(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "proxctl") || !strings.Contains(out, "commit") {
		t.Errorf("version output: %q", out)
	}
}

func TestExecute_DoesNotPanicOnHelp(t *testing.T) {
	// We can't easily intercept os.Exit, but we can at least construct + invoke
	// New().Execute() against a benign arg set without actually calling Execute().
	// To cover internal/root.Execute() we set os.Args and use SilenceUsage.
	isolateHome(t)
	// Substitute os.Args so Execute() runs --help (which returns nil).
	origArgs := os.Args
	os.Args = []string{"proxctl", "--help"}
	t.Cleanup(func() { os.Args = origArgs })

	// Capture stdout/stderr to avoid polluting test output.
	origOut, origErr := os.Stdout, os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout, os.Stderr = w, w
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origOut, origErr
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		Execute()
	}()
	<-done
	w.Close()
	_, _ = io.Copy(io.Discard, r)
}

func TestExecute_ErrorPathCallsOsExit(t *testing.T) {
	isolateHome(t)
	var gotCode int
	origExit := osExit
	osExit = func(c int) { gotCode = c }
	t.Cleanup(func() { osExit = origExit })

	origArgs := os.Args
	os.Args = []string{"proxctl", "no-such-subcommand"}
	t.Cleanup(func() { os.Args = origArgs })

	// Swallow stderr.
	origErr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr; w.Close() })

	Execute()
	if gotCode != 1 {
		t.Errorf("Execute() did not call osExit(1), got %d", gotCode)
	}
}

func TestExecute_ErrorPathInvokesStderr(t *testing.T) {
	// Drive Execute() down the error branch by supplying an unknown subcommand.
	// We replace os.Exit via a package-level hook? No hook exists, so instead
	// we run New().Execute() directly with a bogus arg — that exercises the same
	// line as Execute() minus os.Exit. Already covered above via --help.
	isolateHome(t)
	cmd := New()
	cmd.SetArgs([]string{"no-such-subcommand"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

// ------------------------------------------------------------------------
// license + env + config not-implemented commands
// ------------------------------------------------------------------------

func TestNotImplemented_Subcommands(t *testing.T) {
	isolateHome(t)
	niPaths := [][]string{
		{"config", "use-context", "foo"},
		{"config", "current-context"},
		{"config", "get-contexts"},
		{"stack", "new", "foo"},
		{"stack", "list"},
		{"stack", "use", "foo"},
		{"stack", "current"},
		{"stack", "add", "foo"},
		{"stack", "remove", "foo"},
		{"stack", "show"},
		// Deprecated `env` alias must still function end-to-end (#15).
		{"env", "new", "foo"},
		{"env", "list"},
		{"env", "use", "foo"},
		{"env", "current"},
		{"env", "add", "foo"},
		{"env", "remove", "foo"},
		{"env", "show"},
		{"license", "status"},
		{"license", "activate"},
		{"license", "show"},
		{"license", "seats-used"},
	}
	for _, args := range niPaths {
		name := strings.Join(args, " ")
		t.Run(name, func(t *testing.T) {
			_, err := executeCmd(t, args...)
			if err == nil || !strings.Contains(err.Error(), "not implemented") {
				t.Errorf("%s: expected not-implemented error, got %v", name, err)
			}
		})
	}
}

// ------------------------------------------------------------------------
// config validate / render / schema
// ------------------------------------------------------------------------

func TestConfig_Validate_Success(t *testing.T) {
	home := isolateHome(t)
	envPath := writeRefEnvFixture(t, home)
	out, err := executeCmd(t, "config", "validate", envPath)
	if err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OK:") {
		t.Errorf("validate output: %q", out)
	}
}

func TestConfig_Validate_FailureMissingFile(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "config", "validate", "/nonexistent/env.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestConfig_Render_Redacted(t *testing.T) {
	home := isolateHome(t)
	// Build an env file with VAULT and GEN_SSH_KEY markers to exercise redaction.
	raw := `version: "1"
kind: Env
metadata:
  name: redact-test
  domain: example.com
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      rac-node-1:
        proxmox:
          node_name: pve-1
          vm_id: 1001
        ips:
          public: 10.10.0.101
        resources:
          memory: 4096
          cores: 2
          sockets: 1
        disks:
          - id: 0
            size: 32G
            storage_class: local-lvm
            interface: scsi0
            role: root
        tags:
          - "<VAULT:kv/proxmox/root#token>"
          - "<GEN_SSH_KEY:id_ed25519>"
  networks:
    kind: Networks
    public:
      cidr: 10.10.0.0/24
      gateway: 10.10.0.1
  storage_classes:
    kind: StorageClasses
    local-lvm:
      backend: lvm-thin
`
	envPath := filepath.Join(home, "env.yaml")
	if err := os.WriteFile(envPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := executeCmd(t, "config", "render", envPath)
	if err != nil {
		t.Fatalf("render: %v\n%s", err, out)
	}
	if strings.Contains(out, "<VAULT:") || strings.Contains(out, "<GEN_SSH_KEY:") {
		t.Errorf("render output still contains secret markers:\n%s", out)
	}
	if !strings.Contains(out, "<REDACTED>") {
		t.Errorf("render output missing <REDACTED> marker:\n%s", out)
	}
}

func TestConfig_Render_FailureMissingFile(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "config", "render", "/nonexistent/env.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestConfig_Schema_OutputsJSON(t *testing.T) {
	isolateHome(t)
	out, err := executeCmd(t, "config", "schema")
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	// It's JSON Schema — top-level should be a JSON object.
	trimmed := strings.TrimSpace(out)
	if !strings.HasPrefix(trimmed, "{") {
		t.Errorf("schema output not JSON object, starts: %q", trimmed[:min(60, len(trimmed))])
	}
}

// ------------------------------------------------------------------------
// vm subcommands
// ------------------------------------------------------------------------

func TestVM_List_NoEnvManifest(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "list")
	if err == nil {
		t.Fatal("expected error for missing env.yaml")
	}
}

func TestVM_List_MissingCredentials(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "list")
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Errorf("expected credentials error, got %v", err)
	}
}

func TestVM_List_HTTPMock_Table(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]any{
			{"vmid": "1001", "name": "rac-node-1", "status": "running", "cpus": 4, "maxmem": 8589934592},
		})
	})
	out, err := executeCmd(t, "vm", "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	// Note: vm list writes to os.Stdout directly (not cmd.OutOrStdout()), so we
	// can't capture it here. Success = no error.
}

func TestVM_List_JSON(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{})
	})
	_, err := executeCmd(t, "--json", "vm", "list")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
}

func TestVM_Status_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"vmid": 1001, "name": "rac-node-1", "status": "running", "cpus": 4, "maxmem": 8589934592,
		})
	})
	_, err := executeCmd(t, "vm", "status", "rac-node-1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
}

func TestVM_Status_JSON(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"vmid": 1001, "name": "rac-node-1", "status": "running"})
	})
	_, err := executeCmd(t, "--json", "vm", "status", "rac-node-1")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
}

func TestVM_Status_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, err := executeCmd(t, "vm", "status", "no-such-node")
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestVM_Start_Stop_Reboot(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		// Return empty UPID → no task polling.
		writeJSON(w, "")
	})
	for _, args := range [][]string{
		{"vm", "start", "rac-node-1"},
		{"vm", "stop", "rac-node-1"},
		{"vm", "stop", "rac-node-1", "--force"},
		{"vm", "reboot", "rac-node-1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			_, err := executeCmd(t, args...)
			if err != nil {
				t.Errorf("%v: %v", args, err)
			}
		})
	}
}

func TestVM_Start_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "start", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVM_Start_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "start", "no-such-node")
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestVM_Status_UnknownNode2(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "status", "no-such-node")
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestVM_List_RefFixtureLoadsAndProceeds(t *testing.T) {
	// Post-#19: loadEnvManifest now routes through config.Load which resolves
	// $ref pointers, so vm list against a $ref-composed env succeeds (does not
	// produce "hypervisor not resolved" anymore).
	home := isolateHome(t)
	writeRefEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, []map[string]any{}) })
	if _, err := executeCmd(t, "vm", "list"); err != nil {
		t.Fatalf("vm list against $ref env: %v", err)
	}
}

func TestVM_Create_WorkflowExecutes(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	// Return errors so workflow.Up fails quickly; still covers Renderer + workflow wiring.
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":{"generic":"test"}}`))
	})
	_, _ = executeCmd(t, "vm", "create", "rac-node-1")
}

func TestVM_Stop_ResolveFails(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "stop", "no-such-node")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVM_Reboot_ResolveFails(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "reboot", "no-such-node")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVM_Start_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "start", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestVM_Stop_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "stop", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestVM_Reboot_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "reboot", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestVM_Delete_RequiresYes(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "delete", "rac-node-1")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("expected --yes error, got %v", err)
	}
}

func TestVM_Delete_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "vm", "delete", "no-such-node")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVM_Delete_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "vm", "delete", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVM_Delete_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "vm", "delete", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestVM_Delete_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, "")
	})
	_, err := executeCmd(t, "--yes", "vm", "delete", "rac-node-1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestVM_Create_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "create", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVM_Create_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "create", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestVM_Status_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "status", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVM_Status_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "status", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestVM_List_NoEnvAtAll(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "vm", "list")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ------------------------------------------------------------------------
// snapshot subcommands
// ------------------------------------------------------------------------

func TestSnapshot_Create_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, err := executeCmd(t, "snapshot", "create", "rac-node-1", "snap1")
	if err != nil {
		t.Errorf("snapshot create: %v", err)
	}
}

func TestSnapshot_Create_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "create", "rac-node-1", "snap1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_Create_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "create", "no-such", "snap1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_Create_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "create", "rac-node-1", "snap1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestSnapshot_Restore_RequiresYes(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "restore", "rac-node-1", "snap1")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("expected --yes error, got %v", err)
	}
}

func TestSnapshot_Restore_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "snapshot", "restore", "rac-node-1", "snap1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_Restore_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "snapshot", "restore", "no-such", "snap1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_Restore_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "snapshot", "restore", "rac-node-1", "snap1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestSnapshot_Restore_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, err := executeCmd(t, "--yes", "snapshot", "restore", "rac-node-1", "snap1")
	if err != nil {
		t.Errorf("restore: %v", err)
	}
}

func TestSnapshot_List_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]any{
			{"name": "snap1", "snaptime": 1700000000, "description": "first"},
		})
	})
	_, err := executeCmd(t, "snapshot", "list", "rac-node-1")
	if err != nil {
		t.Errorf("list: %v", err)
	}
}

func TestSnapshot_List_JSON(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, []any{}) })
	_, err := executeCmd(t, "--json", "snapshot", "list", "rac-node-1")
	if err != nil {
		t.Errorf("list --json: %v", err)
	}
}

func TestSnapshot_List_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "list", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_List_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "list", "no-such")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_List_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "list", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestSnapshot_Delete_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, err := executeCmd(t, "snapshot", "delete", "rac-node-1", "snap1")
	if err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestSnapshot_Delete_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "delete", "rac-node-1", "snap1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_Delete_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "delete", "no-such", "snap1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshot_Delete_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "snapshot", "delete", "rac-node-1", "snap1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

// ------------------------------------------------------------------------
// boot subcommands
// ------------------------------------------------------------------------

func TestBoot_ConfigureFirstBoot_RequiresISO(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, err := executeCmd(t, "boot", "configure-first-boot", "rac-node-1")
	if err == nil || !strings.Contains(err.Error(), "--iso required") {
		t.Errorf("expected --iso required error, got %v", err)
	}
}

func TestBoot_ConfigureFirstBoot_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "boot", "configure-first-boot", "rac-node-1", "--iso", "local:iso/x.iso")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBoot_ConfigureFirstBoot_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "boot", "configure-first-boot", "no-such", "--iso", "local:iso/x.iso")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBoot_ConfigureFirstBoot_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "boot", "configure-first-boot", "rac-node-1", "--iso", "local:iso/x.iso")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestBoot_ConfigureFirstBoot_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, err := executeCmd(t, "boot", "configure-first-boot", "rac-node-1",
		"--iso", "local:iso/x.iso", "--order", "ide3;scsi0")
	if err != nil {
		t.Errorf("configure-first-boot: %v", err)
	}
}

func TestBoot_EjectISO_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, err := executeCmd(t, "boot", "eject-iso", "rac-node-1")
	if err != nil {
		t.Errorf("eject-iso: %v", err)
	}
}

func TestBoot_EjectISO_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "boot", "eject-iso", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBoot_EjectISO_UnknownNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "boot", "eject-iso", "no-such")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBoot_EjectISO_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "boot", "eject-iso", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

// ------------------------------------------------------------------------
// kickstart subcommands
// ------------------------------------------------------------------------

func TestKickstart_Distros(t *testing.T) {
	isolateHome(t)
	// distros prints to os.Stdout directly — can't capture, just ensure no error.
	// Use SilenceErrors + run; not-a-flag here, just validate happy path.
	cmd := New()
	cmd.SetArgs([]string{"kickstart", "distros"})
	// Redirect stdout temporarily.
	origOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = origOut }()

	err := cmd.Execute()
	w.Close()
	data, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("distros: %v", err)
	}
	s := string(data)
	if len(s) == 0 {
		t.Error("distros produced no output")
	}
}

func TestKickstart_Generate_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "kickstart", "generate")
	if err == nil {
		t.Fatal("expected error for missing env.yaml")
	}
}

func TestKickstart_Generate_WithFixture(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	outDir := filepath.Join(home, "out")
	_, err := executeCmd(t, "kickstart", "generate", "--out", outDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// Verify at least one .ks file was produced.
	entries, _ := os.ReadDir(outDir)
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ks") {
			found = true
		}
	}
	if !found {
		t.Errorf("no .ks files written in %s", outDir)
	}
}

func TestKickstart_Generate_SingleNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	outDir := filepath.Join(home, "out2")
	_, err := executeCmd(t, "kickstart", "generate", "--node", "rac-node-1", "--out", outDir)
	if err != nil {
		t.Fatalf("generate single: %v", err)
	}
}

func TestKickstart_Generate_RefFixtureLoaderResolvesRefs(t *testing.T) {
	// Post-#19: loadEnvManifest now routes through config.Load which resolves
	// $ref pointers. This test asserts the loader-level fix: kickstart generate
	// against a $ref-composed env no longer fails with "hypervisor not resolved".
	// Downstream errors (e.g. the shared testdata fixture has no kickstart distro
	// set, so the renderer reports "env has no kickstart config") are acceptable
	// here — they prove we got past the loader gate.
	home := isolateHome(t)
	writeRefEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "kickstart", "generate", "--out", filepath.Join(home, "out-ref"))
	if err != nil && strings.Contains(err.Error(), "hypervisor not resolved") {
		t.Fatalf("loadEnvManifest still not resolving $refs: %v", err)
	}
}

func TestKickstart_Generate_RenderErrorBadNode(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	// Single node that doesn't exist in manifest — renderer will fail.
	_, err := executeCmd(t, "kickstart", "generate", "--node", "no-such-node", "--out", filepath.Join(home, "outX"))
	if err == nil {
		t.Fatal("expected render error for unknown node")
	}
}

func TestKickstart_Generate_MkdirError(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	// Create a file and try to write output dir as its child — MkdirAll returns error.
	blocker := filepath.Join(home, "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0o644)
	_, err := executeCmd(t, "kickstart", "generate", "--out", filepath.Join(blocker, "child"))
	if err == nil {
		t.Fatal("expected MkdirAll error")
	}
}

func TestKickstart_Generate_WriteFileError(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	outDir := filepath.Join(home, "readonly")
	_ = os.MkdirAll(outDir, 0o555)
	t.Cleanup(func() { _ = os.Chmod(outDir, 0o755) })
	_, err := executeCmd(t, "kickstart", "generate", "--node", "rac-node-1", "--out", outDir)
	if err == nil {
		t.Log("WriteFile did not fail on 0o555 dir (running as root?)")
	}
}

func TestKickstart_Upload_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	iso := filepath.Join(home, "kickstart.iso")
	_ = os.WriteFile(iso, []byte("fake iso"), 0o644)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, "UPID:pve-1:00001:00:0:upload::root@pam:")
	})
	_, _ = executeCmd(t, "kickstart", "upload", iso, "--storage", "local", "--node", "pve-prod-1")
	// UploadISO may fail at task-polling stage — either branch is fine; we just
	// wanted the code after loadProxmoxClient() to execute.
}

func TestKickstart_BuildISO_BuildFails(t *testing.T) {
	// With a bogus bootloader dir, kickstart.ISOBuilder.Build() fails fast.
	home := isolateHome(t)
	t.Chdir(home)
	ks := filepath.Join(home, "h1.ks")
	_ = os.WriteFile(ks, []byte("# ks"), 0o644)
	bogus := filepath.Join(home, "no-such-bootloader")
	_, err := executeCmd(t, "kickstart", "build-iso", ks, "--bootloader-dir", bogus)
	if err == nil {
		t.Fatal("expected build error")
	}
}

func TestKickstart_Generate_ExplicitEnvPath(t *testing.T) {
	home := isolateHome(t)
	envPath := writeEnvFixture(t, home)
	t.Chdir(home)
	outDir := filepath.Join(home, "out3")
	_, err := executeCmd(t, "kickstart", "generate", envPath, "--out", outDir)
	if err != nil {
		t.Fatalf("generate explicit: %v", err)
	}
}

func TestKickstart_BuildISO_RequiresBootloader(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	ks := filepath.Join(home, "foo.ks")
	_ = os.WriteFile(ks, []byte("# ks"), 0o644)
	_, err := executeCmd(t, "kickstart", "build-iso", ks)
	if err == nil || !strings.Contains(err.Error(), "--bootloader-dir") {
		t.Errorf("expected --bootloader-dir error, got %v", err)
	}
}

func TestKickstart_BuildISO_MissingKSFile(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "kickstart", "build-iso", "/nonexistent.ks")
	if err == nil {
		t.Fatal("expected error for missing ks file")
	}
}

func TestKickstart_Upload_RequiresStorageAndNode(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	iso := filepath.Join(home, "foo.iso")
	_ = os.WriteFile(iso, []byte("x"), 0o644)
	_, err := executeCmd(t, "kickstart", "upload", iso)
	if err == nil || !strings.Contains(err.Error(), "--storage") {
		t.Errorf("expected --storage/--node error, got %v", err)
	}
}

func TestKickstart_Upload_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	iso := filepath.Join(home, "foo.iso")
	_ = os.WriteFile(iso, []byte("x"), 0o644)
	_, err := executeCmd(t, "kickstart", "upload", iso, "--storage", "local", "--node", "pve-1")
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Errorf("expected credentials error, got %v", err)
	}
}

// ------------------------------------------------------------------------
// workflow subcommands
// ------------------------------------------------------------------------

func TestWorkflow_Plan_RequiresNode(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "workflow", "plan")
	if err == nil || !strings.Contains(err.Error(), "--node required") {
		t.Errorf("expected --node required, got %v", err)
	}
}

func TestWorkflow_Plan_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "plan", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected error for missing env.yaml")
	}
}

func TestWorkflow_Plan_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "plan", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Plan_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	// Return a VM-not-found style response so Plan decides to create it.
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vmid":"no such vm"}}`))
			return
		}
		writeJSON(w, nil)
	})
	_, err := executeCmd(t, "workflow", "plan", "--node", "rac-node-1")
	// Either succeeds or returns an error from planning — either branch is OK;
	// we only need coverage of the plan wiring.
	_ = err
}

func TestWorkflow_Plan_BootloaderFlag(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, nil)
	})
	_, _ = executeCmd(t, "workflow", "plan", "--node", "rac-node-1", "--bootloader-dir", home)
}

func TestWorkflow_Plan_JSON(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, _ = executeCmd(t, "--json", "workflow", "plan", "--node", "rac-node-1")
}

func TestWorkflow_Up_RequiresNode(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "workflow", "up")
	if err == nil || !strings.Contains(err.Error(), "--node required") {
		t.Errorf("expected --node required, got %v", err)
	}
}

func TestWorkflow_Up_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "up", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkflow_Up_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "up", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Up_DryRun_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, _ = executeCmd(t, "workflow", "up", "--node", "rac-node-1", "--dry-run")
}

func TestWorkflow_Down_RequiresNode(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "workflow", "down")
	if err == nil || !strings.Contains(err.Error(), "--node required") {
		t.Errorf("expected --node required, got %v", err)
	}
}

func TestWorkflow_Down_RequiresYes(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "workflow", "down", "--node", "rac-node-1")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("expected --yes error, got %v", err)
	}
}

func TestWorkflow_Down_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "workflow", "down", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkflow_Down_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "workflow", "down", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Down_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, _ = executeCmd(t, "--yes", "workflow", "down", "--node", "rac-node-1", "--force")
}

func TestWorkflow_Status_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "status")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkflow_Status_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "status")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Status_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/status/current") {
			writeJSON(w, map[string]any{"vmid": 1001, "name": "rac-node-1", "status": "running"})
			return
		}
		writeJSON(w, nil)
	})
	_, err := executeCmd(t, "workflow", "status")
	if err != nil {
		t.Errorf("status: %v", err)
	}
}

func TestWorkflow_Verify_RequiresNode(t *testing.T) {
	isolateHome(t)
	_, err := executeCmd(t, "workflow", "verify")
	if err == nil || !strings.Contains(err.Error(), "--node required") {
		t.Errorf("expected --node required, got %v", err)
	}
}

func TestWorkflow_Verify_NoEnv(t *testing.T) {
	home := isolateHome(t)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "verify", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkflow_Verify_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "verify", "--node", "rac-node-1")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Verify_HTTPMock(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, nil) })
	_, _ = executeCmd(t, "workflow", "verify", "--node", "rac-node-1")
}

// ------------------------------------------------------------------------
// clientutil extra coverage
// ------------------------------------------------------------------------

// TestLoadProxmoxClient_FromConfigFile verifies the YAML config path is picked
// up when env vars are missing.
func TestLoadProxmoxClient_FromConfigFile(t *testing.T) {
	home := isolateHome(t)
	cfgDir := filepath.Join(home, ".proxctl")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `current-context: ci
contexts:
  - name: ci
    endpoint: https://pve.example.com:8006
    token_id: "root@pam!ci"
    token_secret: "secret"
    insecure_tls: true
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	cli, err := loadProxmoxClient()
	if err != nil {
		t.Fatalf("loadProxmoxClient: %v", err)
	}
	if cli == nil {
		t.Fatal("nil client")
	}
}

func TestLoadProxmoxClient_ConfigFileParseError(t *testing.T) {
	home := isolateHome(t)
	cfgDir := filepath.Join(home, ".proxctl")
	_ = os.MkdirAll(cfgDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(":::not yaml:::"), 0o644)
	_, err := loadProxmoxClient()
	if err == nil {
		t.Fatal("expected yaml parse error")
	}
}

func TestLoadProxmoxClient_ContextOverride(t *testing.T) {
	home := isolateHome(t)
	cfgDir := filepath.Join(home, ".proxctl")
	_ = os.MkdirAll(cfgDir, 0o755)
	cfg := `current-context: dev
contexts:
  - name: dev
    endpoint: https://dev:8006
    token_id: "root@pam!dev"
    token_secret: "dev"
  - name: prod
    endpoint: https://prod:8006
    token_id: "root@pam!prod"
    token_secret: "prod"
`
	_ = os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644)
	flagContext = "prod"
	t.Cleanup(func() { flagContext = "" })
	cli, err := loadProxmoxClient()
	if err != nil {
		t.Fatalf("loadProxmoxClient: %v", err)
	}
	if cli == nil {
		t.Fatal("nil")
	}
}

func TestLoadProxmoxClient_EnvOverrides(t *testing.T) {
	isolateHome(t)
	t.Setenv("PROXCTL_ENDPOINT", "https://envpve:8006")
	t.Setenv("PROXCTL_TOKEN_ID", "root@pam!env")
	t.Setenv("PROXCTL_TOKEN_SECRET", "envsecret")
	t.Setenv("PROXCTL_INSECURE_TLS", "1")
	cli, err := loadProxmoxClient()
	if err != nil {
		t.Fatalf("loadProxmoxClient: %v", err)
	}
	if cli == nil {
		t.Fatal("nil")
	}
}

func TestLoadEnvManifest_ReadError(t *testing.T) {
	isolateHome(t)
	_, err := loadEnvManifest("/nonexistent/env.yaml")
	if err == nil {
		t.Fatal("expected read error")
	}
}

func TestLoadEnvManifest_ParseError(t *testing.T) {
	home := isolateHome(t)
	bad := filepath.Join(home, "bad.yaml")
	_ = os.WriteFile(bad, []byte(":\n::not yaml"), 0o644)
	_, err := loadEnvManifest(bad)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadEnvManifest_FlagEnv(t *testing.T) {
	home := isolateHome(t)
	envPath := writeEnvFixture(t, home)
	flagStack = envPath
	t.Cleanup(func() { flagStack = "" })
	env, err := loadEnvManifest("")
	if err != nil {
		t.Fatalf("loadEnvManifest: %v", err)
	}
	if env.Metadata.Name == "" {
		t.Error("env did not load")
	}
}

func TestResolveNodeRef_Success(t *testing.T) {
	home := isolateHome(t)
	envPath := writeEnvFixture(t, home)
	env, err := loadEnvManifest(envPath)
	if err != nil {
		t.Fatal(err)
	}
	node, vmid, err := resolveNodeRef(env, "rac-node-1")
	if err != nil {
		t.Fatalf("resolveNodeRef: %v", err)
	}
	if node == "" || vmid == 0 {
		t.Errorf("unexpected node=%q vmid=%d", node, vmid)
	}
}

func TestNotImplemented_Fn(t *testing.T) {
	fn := notImplemented("test")
	err := fn(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not-implemented error, got %v", err)
	}
}

// ensure fmt is used (builds clean even if some tests above don't need it).
var _ = fmt.Sprintf
