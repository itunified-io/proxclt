package root

import (
	"strings"
	"testing"

	"github.com/itunified-io/proxctl/pkg/config"
)

func TestWorkflowProfile_List(t *testing.T) {
	out, err := executeCmd(t, "workflow", "profile", "list")
	if err != nil {
		t.Fatalf("profile list: %v", err)
	}
	for _, want := range []string{"oracle-rac-2node", "pg-single", "host-only"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in list output, got:\n%s", want, out)
		}
	}
}

func TestWorkflowProfile_List_JSON(t *testing.T) {
	out, err := executeCmd(t, "--json", "workflow", "profile", "list")
	if err != nil {
		t.Fatalf("profile list --json: %v", err)
	}
	if !strings.Contains(out, "oracle-rac-2node") {
		t.Errorf("json list missing profile: %s", out)
	}
}

func TestWorkflowProfile_Show(t *testing.T) {
	out, err := executeCmd(t, "workflow", "profile", "show", "pg-single")
	if err != nil {
		t.Fatalf("profile show: %v", err)
	}
	if !strings.Contains(out, "pg-single") {
		t.Errorf("expected profile content, got:\n%s", out)
	}
}

func TestWorkflowProfile_Show_Missing(t *testing.T) {
	_, err := executeCmd(t, "workflow", "profile", "show", "no-such-profile")
	if err == nil {
		t.Fatal("want error for missing profile")
	}
}

func TestIsMultiNode_NilInputs(t *testing.T) {
	if isMultiNode(nil) {
		t.Error("nil env should be single-node")
	}
	// Env present but hypervisor unresolved.
	env := &config.Env{Version: "1", Kind: "Env"}
	if isMultiNode(env) {
		t.Error("unresolved hypervisor should be single-node")
	}
	// Single-node env → not multi.
	hyp := &config.Hypervisor{Kind: "Hypervisor", Nodes: map[string]config.Node{
		"only": {Proxmox: config.ProxmoxRef{NodeName: "pve", VMID: 1}},
	}}
	env.Spec.Hypervisor.Inline = hyp
	env.Spec.Hypervisor.Value = hyp
	if isMultiNode(env) {
		t.Error("single-node should not be multi")
	}
}
