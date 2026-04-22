package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeFileImpl(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// Crafted YAML nodes to hit unmarshal non-scalar-key and bad-kind branches.

// makeMappingWithNonScalarKey builds a mapping node with one non-scalar key +
// one valid scalar key, which exercises the `k.Kind != ScalarNode: continue`
// branch in the custom UnmarshalYAML implementations.
func makeMappingWithNonScalarKey() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "seq"},
				{Kind: yaml.ScalarNode, Value: "key"},
			}}, // non-scalar key (a mapping)
			{Kind: yaml.ScalarNode, Value: "ignored"},
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.ScalarNode, Value: "Linux"},
		},
	}
}

func TestLinux_NonScalarKeyIgnored(t *testing.T) {
	var l Linux
	if err := makeMappingWithNonScalarKey().Decode(&l); err != nil {
		t.Fatal(err)
	}
	if l.Kind != "Linux" {
		t.Errorf("kind: %q", l.Kind)
	}
}

func TestHook_NonScalarKeyIgnored(t *testing.T) {
	// Hook mapping with non-scalar key + type scalar.
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "k"},
				{Kind: yaml.ScalarNode, Value: "v"},
			}},
			{Kind: yaml.ScalarNode, Value: "x"},
			{Kind: yaml.ScalarNode, Value: "type"},
			{Kind: yaml.ScalarNode, Value: "slack"},
		},
	}
	var h Hook
	if err := n.Decode(&h); err != nil {
		t.Fatal(err)
	}
	if h.Type != "slack" {
		t.Errorf("type: %q", h.Type)
	}
}

func TestDatabase_NonScalarKeyIgnored(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "k"}, {Kind: yaml.ScalarNode, Value: "v"},
			}},
			{Kind: yaml.ScalarNode, Value: "x"},
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.ScalarNode, Value: "OracleDatabase"},
		},
	}
	var d Database
	if err := n.Decode(&d); err != nil {
		t.Fatal(err)
	}
	if d.Kind != "OracleDatabase" {
		t.Errorf("kind: %q", d.Kind)
	}
}

func TestNetworks_NonScalarKeyIgnored(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "k"}, {Kind: yaml.ScalarNode, Value: "v"},
			}},
			{Kind: yaml.ScalarNode, Value: "x"},
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.ScalarNode, Value: "Networks"},
			{Kind: yaml.ScalarNode, Value: "pub"},
			{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "cidr"}, {Kind: yaml.ScalarNode, Value: "10.0.0.0/24"},
			}},
		},
	}
	var net Networks
	if err := n.Decode(&net); err != nil {
		t.Fatal(err)
	}
	if net.Kind != "Networks" {
		t.Errorf("kind: %q", net.Kind)
	}
}

func TestStorageClasses_NonScalarKeyIgnored(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "k"}, {Kind: yaml.ScalarNode, Value: "v"},
			}},
			{Kind: yaml.ScalarNode, Value: "x"},
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.ScalarNode, Value: "StorageClasses"},
			{Kind: yaml.ScalarNode, Value: "local"},
			{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "backend"}, {Kind: yaml.ScalarNode, Value: "lvm"},
			}},
		},
	}
	var sc StorageClasses
	if err := n.Decode(&sc); err != nil {
		t.Fatal(err)
	}
	if sc.Kind != "StorageClasses" {
		t.Errorf("kind: %q", sc.Kind)
	}
}

// Non-scalar "kind" value must error for each container kind.

func TestLinux_KindMustBeScalar(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.MappingNode}, // non-scalar value for kind
		},
	}
	var l Linux
	if err := n.Decode(&l); err == nil {
		t.Fatal("want error")
	}
}

func TestHook_TypeMustBeScalar(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "type"},
			{Kind: yaml.MappingNode},
		},
	}
	var h Hook
	if err := n.Decode(&h); err == nil {
		t.Fatal("want error")
	}
}

func TestDatabase_KindMustBeScalar(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.MappingNode},
		},
	}
	var d Database
	if err := n.Decode(&d); err == nil {
		t.Fatal("want error")
	}
}

func TestNetworks_KindMustBeScalar(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.MappingNode},
		},
	}
	var net Networks
	if err := n.Decode(&net); err == nil {
		t.Fatal("want error")
	}
}

func TestStorageClasses_KindMustBeScalar(t *testing.T) {
	n := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "kind"},
			{Kind: yaml.MappingNode},
		},
	}
	var sc StorageClasses
	if err := n.Decode(&sc); err == nil {
		t.Fatal("want error")
	}
}

// --- Ref[T].MarshalYAML with no content ----------------------------------

// Reach walkStrings line 243-245: invalid zero Value input.
func TestWalkStrings_InvalidValue(t *testing.T) {
	// Pass untyped nil — walkStrings recovers gracefully.
	WalkStringsForTest(nil, func(s string) string { return s })
}

// Reach transformMapValue default branch (non-string, non-interface, mutable).
func TestResolve_MapWithStructValue(t *testing.T) {
	type inner struct{ V string }
	m := map[string]inner{"a": {V: "${env:K}"}}
	ResolvePlaceholders(&m, ResolverOpts{LookupEnv: func(string) (string, bool) { return "x", true }})
	if m["a"].V != "x" {
		t.Errorf("struct in map not expanded: %+v", m)
	}
}

// Reach loader's Validate error return (line 63-65).
func TestLoad_ValidationFails(t *testing.T) {
	dir := t.TempDir()
	// Unknown network reference triggers cross-field validator.
	content := `version: "1"
kind: Env
metadata: {name: bad-xref}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1:
        proxmox: {node_name: pve, vm_id: 100}
        ips: {public: 10.0.0.1}
        nics:
          - {name: net0, usage: public, network: nope, ipv4: {address: 10.0.0.1/24}}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	p := dir + "/env.yaml"
	if err := writeFile(p, content); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want validation failure")
	}
}

func writeFile(path, content string) error {
	return writeFileImpl(path, content)
}

// Test resolveRefPath final deref + IsNil branches (lines 218-226).
func TestResolveRefPath_FinalPtrDeref(t *testing.T) {
	type inner struct{ Name string }
	type root struct{ Ptr *inner }
	// Ptr present → final pointer loop derefs to struct, returns %v of struct.
	r := &root{Ptr: &inner{Name: "hi"}}
	got, err := resolveRefPath(r, "Ptr")
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Errorf("final deref: %q", got)
	}
	// Nil Ptr → final loop triggers "nil value" error.
	r2 := &root{}
	if _, err := resolveRefPath(r2, "Ptr"); err == nil {
		t.Fatal("want nil value error")
	}
}

func TestRef_MarshalEmpty_ReturnsNil(t *testing.T) {
	var r Ref[Cluster]
	v, err := r.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Errorf("want nil got %v", v)
	}
}
