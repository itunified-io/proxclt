package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnv_UnmarshalInline(t *testing.T) {
	y := `
version: "1"
kind: Env
metadata:
  name: test
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1:
        proxmox: {node_name: pve, vm_id: 100}
        ips: {public: 10.0.0.1}
  networks:
    kind: Networks
    public:
      cidr: 10.0.0.0/24
  storage_classes:
    kind: StorageClasses
    local:
      backend: lvm
`
	var env Env
	if err := yaml.Unmarshal([]byte(y), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Version != "1" || env.Kind != "Env" || env.Metadata.Name != "test" {
		t.Errorf("metadata mismatch: %+v", env.Metadata)
	}
	if env.Spec.Hypervisor.Inline == nil {
		t.Fatal("hypervisor should be inline")
	}
	if env.Spec.Networks.Inline == nil || env.Spec.Networks.Inline.Kind != "Networks" {
		t.Errorf("networks inline: %+v", env.Spec.Networks.Inline)
	}
	if _, ok := env.Spec.Networks.Inline.Zones["public"]; !ok {
		t.Errorf("missing public zone in networks: %+v", env.Spec.Networks.Inline.Zones)
	}
}

func TestEnv_UnmarshalRef(t *testing.T) {
	y := `
version: "1"
kind: Env
metadata: {name: test}
spec:
  hypervisor: {$ref: ./hypervisor.yaml}
  networks: {$ref: ./networks.yaml}
  storage_classes: {$ref: ./sc.yaml}
`
	var env Env
	if err := yaml.Unmarshal([]byte(y), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Spec.Hypervisor.Ref != "./hypervisor.yaml" {
		t.Errorf("ref mismatch: %q", env.Spec.Hypervisor.Ref)
	}
}

func TestEnv_MarshalRoundtrip(t *testing.T) {
	env := Env{
		Version: "1",
		Kind:    "Env",
		Metadata: EnvMetadata{Name: "roundtrip"},
		Spec: EnvSpec{
			Hypervisor: Ref[Hypervisor]{Inline: &Hypervisor{
				Kind: "Hypervisor",
				Nodes: map[string]Node{
					"n1": {
						Proxmox: ProxmoxRef{NodeName: "pve", VMID: 100},
						IPs:     map[string]string{"public": "10.0.0.1"},
					},
				},
			}},
			Networks: Ref[Networks]{Inline: &Networks{
				Kind:  "Networks",
				Zones: map[string]NetworkZone{"public": {CIDR: "10.0.0.0/24"}},
			}},
			StorageClasses: Ref[StorageClasses]{Inline: &StorageClasses{
				Kind:    "StorageClasses",
				Classes: map[string]StorageClass{"local": {Backend: "lvm"}},
			}},
		},
	}
	b, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), "kind: Env") {
		t.Errorf("marshalled output missing kind: %s", b)
	}
}
