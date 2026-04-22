package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func decodeYAML(t *testing.T, b []byte, v any) error {
	t.Helper()
	return yaml.Unmarshal(b, v)
}

// --- Validate: errors surfacing from resolved-child validation ------------
//
// Each test crafts a scenario where the child's struct passes the top-level
// walk but exposes a tag failure on the resolved child (so the "child:"
// wrapping error branch fires). Because struct validation walks everywhere
// regardless, we can simply trigger invalid child fields.

func TestValidate_Hypervisor_BadKind(t *testing.T) {
	env := baseEnv()
	env.Spec.Hypervisor.Value.Kind = "Wrong"
	if err := Validate(env); err == nil {
		t.Fatal("want hypervisor kind error")
	}
}

func TestValidate_Networks_BadKind(t *testing.T) {
	env := baseEnv()
	env.Spec.Networks.Value.Kind = "Wrong"
	if err := Validate(env); err == nil {
		t.Fatal("want networks kind error")
	}
}

func TestValidate_StorageClasses_BadKind(t *testing.T) {
	env := baseEnv()
	env.Spec.StorageClasses.Value.Kind = "Wrong"
	if err := Validate(env); err == nil {
		t.Fatal("want sc kind error")
	}
}

func TestValidate_Cluster_BadKind(t *testing.T) {
	env := baseEnv()
	env.Spec.Cluster = &Ref[Cluster]{Value: &Cluster{Kind: "Wrong", Type: "plain"}}
	if err := Validate(env); err == nil {
		t.Fatal("want cluster kind error")
	}
}

func TestValidate_Linux_BadKind(t *testing.T) {
	env := baseEnv()
	env.Spec.Linux = &Ref[Linux]{Value: &Linux{Kind: "Wrong"}}
	if err := Validate(env); err == nil {
		t.Fatal("want linux kind error")
	}
}

func TestValidate_Database_BadKind(t *testing.T) {
	env := baseEnv()
	env.Spec.Databases = []Ref[Database]{{Value: &Database{Kind: "Wrong"}}}
	if err := Validate(env); err == nil {
		t.Fatal("want database kind error")
	}
}

// --- transformMapValue: non-string + nil interface branches --------------

func TestResolve_MapWithNestedStruct(t *testing.T) {
	// map[string]any pointing at a struct with a placeholder string.
	type inner struct{ V string }
	root := map[string]any{
		"a": inner{V: "${env:K}"},
		"b": "${env:K}",
		"n": nil,
	}
	ResolvePlaceholders(&root, ResolverOpts{LookupEnv: func(k string) (string, bool) {
		return "expanded", true
	}})
	if root["b"].(string) != "expanded" {
		t.Errorf("top map string: %v", root["b"])
	}
}

func TestResolve_MapIntValue_Untouched(t *testing.T) {
	root := map[string]any{"n": 42, "s": "${env:X}"}
	ResolvePlaceholders(&root, ResolverOpts{LookupEnv: func(k string) (string, bool) { return "v", true }})
	if root["n"].(int) != 42 {
		t.Errorf("int mangled: %v", root["n"])
	}
	if root["s"].(string) != "v" {
		t.Errorf("str not expanded: %v", root["s"])
	}
}

// --- Loader resolveRefs error paths --------------------------------------

func TestLoad_BadNetworksRef(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
metadata: {name: n}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
  networks: {$ref: ./missing.yaml}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want networks ref error")
	}
}

func TestLoad_BadStorageClassesRef(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
metadata: {name: n}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {$ref: ./missing.yaml}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want sc ref error")
	}
}

func TestLoad_BadLinuxRef(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
metadata: {name: n}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
  linux: {$ref: ./missing.yaml}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want linux ref error")
	}
}

func TestLoad_BadClusterRef(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
metadata: {name: n}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
  cluster: {$ref: ./missing.yaml}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want cluster ref error")
	}
}

func TestLoad_BadDatabaseRef(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
metadata: {name: n}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
  databases:
    - {$ref: ./missing-db.yaml}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want database ref error")
	}
}

// --- Additional Env UnmarshalYAML coverage ---------------------------------

func TestRef_UnmarshalMultiKeyWithDollarRef(t *testing.T) {
	// yaml has multiple keys; one is $ref. The multi-key branch fires.
	// We can't use standard YAML because $ref as a key among multiple keys
	// is still a mapping node; verify it picks up $ref.
	content := []byte(`
$ref: ./x.yaml
extra: ignored
`)
	var r Ref[Cluster]
	// Decode via yaml.
	if err := decodeYAML(t, content, &r); err != nil {
		t.Fatal(err)
	}
	if r.Ref != "./x.yaml" {
		t.Errorf("multi-key ref: %q", r.Ref)
	}
}
