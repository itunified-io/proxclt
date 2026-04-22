package root

import (
	"net/http"
	"strings"
	"testing"
)

// TestWorkflow_Plan_MultiNodeDispatch exercises the len(Nodes)>1 branch of
// the plan subcommand — no --node flag needed; the plan aggregates per node.
func TestWorkflow_Plan_MultiNodeDispatch(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vmid":"no such vm"}}`))
			return
		}
		writeJSON(w, nil)
	})
	out, err := executeCmd(t, "workflow", "plan")
	if err != nil {
		t.Fatalf("multi-node plan: %v", err)
	}
	// Multi-node plan output lists NODE column.
	if !strings.Contains(out, "NODE") {
		t.Errorf("expected NODE column in multi-node plan, got:\n%s", out)
	}
}

func TestWorkflow_Plan_MultiNode_JSON(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vmid":"no such vm"}}`))
			return
		}
		writeJSON(w, nil)
	})
	_, err := executeCmd(t, "--json", "workflow", "plan")
	if err != nil {
		t.Fatalf("multi-node plan --json: %v", err)
	}
}

func TestWorkflow_Verify_MultiNodeDispatch(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			writeJSON(w, map[string]any{"vmid": 1001, "status": "running"})
			return
		}
		writeJSON(w, nil)
	})
	// Multi-node verify may fail (verify asserts "running" for each VM) but we
	// only care that the multi-node dispatch path executes.
	_, _ = executeCmd(t, "workflow", "verify")
}

func TestWorkflow_Up_MultiNode_MaxConcurrency(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vmid":"no such vm"}}`))
			return
		}
		writeJSON(w, nil)
	})
	// Exercise --max-concurrency and --continue-on-error flags (covers
	// makeMulti's conditional field assignments).
	_, _ = executeCmd(t, "workflow", "up", "--dry-run", "--max-concurrency", "2", "--continue-on-error")
}

func TestWorkflow_Up_MultiNode_DryRun(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vmid":"no such vm"}}`))
			return
		}
		writeJSON(w, nil)
	})
	_, _ = executeCmd(t, "workflow", "up", "--dry-run")
}

func TestWorkflow_Plan_MultiNode_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "plan")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Plan_MultiNode_PlanError(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	// VM exists → Plan aborts for both nodes.
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			writeJSON(w, map[string]any{"vmid": 1001, "status": "running"})
			return
		}
		writeJSON(w, nil)
	})
	_, err := executeCmd(t, "workflow", "plan")
	if err == nil {
		t.Error("want plan error when VMs already exist")
	}
}

func TestWorkflow_Up_MultiNode_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "up")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Verify_MultiNode_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "workflow", "verify")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Down_MultiNode_MissingCreds(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	_, err := executeCmd(t, "--yes", "workflow", "down")
	if err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestWorkflow_Down_MultiNodeDispatch(t *testing.T) {
	home := isolateHome(t)
	writeEnvFixture(t, home)
	t.Chdir(home)
	startProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status/current") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vmid":"no such vm"}}`))
			return
		}
		writeJSON(w, nil)
	})
	_, err := executeCmd(t, "--yes", "workflow", "down")
	if err != nil {
		t.Errorf("multi-node down: %v", err)
	}
}
