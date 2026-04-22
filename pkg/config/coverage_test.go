package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- Resolver edge cases --------------------------------------------------

func TestResolve_FileMissing_Error(t *testing.T) {
	r := &rootStruct{Secret: "${file:/no/such}"}
	res := ResolvePlaceholders(r, ResolverOpts{})
	if len(res.Errors) == 0 {
		t.Fatal("want file error")
	}
}

func TestResolve_FileMissing_WithDefault(t *testing.T) {
	r := &rootStruct{Secret: "${file:/no/such|default:fb}"}
	res := ResolvePlaceholders(r, ResolverOpts{})
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected: %v", res.Errors)
	}
	if r.Secret != "fb" {
		t.Errorf("got %q", r.Secret)
	}
}

func TestResolve_FileHomeExpansion(t *testing.T) {
	// write to tmp, reference via ${file:/abs}
	d := t.TempDir()
	p := filepath.Join(d, "tok")
	if err := os.WriteFile(p, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &rootStruct{Secret: "${file:" + p + "}"}
	res := ResolvePlaceholders(r, ResolverOpts{})
	if len(res.Errors) != 0 {
		t.Fatalf("%v", res.Errors)
	}
	if r.Secret != "from-file" {
		t.Errorf("got %q", r.Secret)
	}
}

func TestResolve_EnvEmptyStringTreatedAsMissing(t *testing.T) {
	r := &rootStruct{Secret: "${env:X|default:fb}"}
	ResolvePlaceholders(r, ResolverOpts{LookupEnv: func(string) (string, bool) { return "", true }})
	if r.Secret != "fb" {
		t.Errorf("empty env should fall back to default: %q", r.Secret)
	}
}

func TestResolve_GenPasswordBad(t *testing.T) {
	// bad N
	r := &rootStruct{Secret: "${gen:password:abc}"}
	ResolvePlaceholders(r, ResolverOpts{EnvName: "x"})
	if r.Secret != "" {
		t.Errorf("want empty on bad N, got %q", r.Secret)
	}
	// zero N
	r2 := &rootStruct{Secret: "${gen:password:0}"}
	ResolvePlaceholders(r2, ResolverOpts{EnvName: "x"})
	if r2.Secret != "" {
		t.Errorf("want empty on zero: %q", r2.Secret)
	}
	// no arg
	if got := expandGen("password", "e"); got != "" {
		t.Errorf("no N: %q", got)
	}
	// huge N clamped to hex length
	got := expandGen("password:9999", "e")
	if len(got) == 0 || len(got) > 64 {
		t.Errorf("clamp: len=%d", len(got))
	}
	// with seed
	g1 := expandGen("password:8:s1", "e")
	g2 := expandGen("password:8:s2", "e")
	if g1 == g2 {
		t.Errorf("different seeds must differ")
	}
	// unknown gen kind
	if got := expandGen("unknown", "e"); got != "" {
		t.Errorf("unknown gen: %q", got)
	}
	// empty spec
	if got := expandGen("", "e"); got != "" {
		t.Errorf("empty spec: %q", got)
	}
}

func TestResolve_SSHKey_DefaultType(t *testing.T) {
	if got := expandGen("ssh_key", "e"); !strings.Contains(got, "ed25519") {
		t.Errorf("default ed25519: %q", got)
	}
	if got := expandGen("ssh_key:rsa", "e"); !strings.Contains(got, "rsa") {
		t.Errorf("rsa: %q", got)
	}
}

func TestResolve_UnknownKind_PassthroughViaRegex(t *testing.T) {
	// The regex whitelists kinds; an unknown kind string just won't match.
	r := &rootStruct{Secret: "${weird:x}"}
	ResolvePlaceholders(r, ResolverOpts{})
	if r.Secret != "${weird:x}" {
		t.Errorf("non-matching placeholder should be untouched: %q", r.Secret)
	}
}

func TestResolve_UnknownFilter_Ignored(t *testing.T) {
	r := &rootStruct{Secret: "${env:Z|nosuch}"}
	ResolvePlaceholders(r, ResolverOpts{LookupEnv: func(string) (string, bool) { return "abc", true }})
	if r.Secret != "abc" {
		t.Errorf("unknown filter should be no-op: %q", r.Secret)
	}
}

func TestResolve_Ref_NoField(t *testing.T) {
	env := &Env{Metadata: EnvMetadata{Name: "e"}}
	env.Metadata.Tags = map[string]string{"x": "${ref:Metadata.NoSuch}"}
	res := ResolvePlaceholders(env, ResolverOpts{})
	if len(res.Errors) == 0 {
		t.Fatal("want ref error")
	}
}

func TestResolve_Ref_Nil(t *testing.T) {
	var env *Env
	_, err := resolveRefPath(env, "Metadata.Name")
	if err == nil {
		t.Fatal("want nil error")
	}
}

func TestResolve_Ref_ThroughMap(t *testing.T) {
	env := &Env{Metadata: EnvMetadata{Name: "e", Tags: map[string]string{"k": "v"}}}
	got, err := resolveRefPath(env, "Metadata.Tags.k")
	if err != nil {
		t.Fatal(err)
	}
	if got != "v" {
		t.Errorf("map traverse: %q", got)
	}
}

func TestResolve_Ref_BadMapKey(t *testing.T) {
	env := &Env{Metadata: EnvMetadata{Name: "e", Tags: map[string]string{"k": "v"}}}
	if _, err := resolveRefPath(env, "Metadata.Tags.nope"); err == nil {
		t.Fatal("want map key error")
	}
}

func TestResolve_Ref_CannotTraverseScalar(t *testing.T) {
	env := &Env{Metadata: EnvMetadata{Name: "e"}}
	if _, err := resolveRefPath(env, "Metadata.Name.Extra"); err == nil {
		t.Fatal("want cannot-traverse error")
	}
}

func TestResolve_WalkStringsIntoSlice(t *testing.T) {
	r := &rootStruct{List: []string{"${env:A}", "static"}}
	ResolvePlaceholders(r, ResolverOpts{LookupEnv: func(k string) (string, bool) {
		if k == "A" {
			return "expanded", true
		}
		return "", false
	}})
	if r.List[0] != "expanded" || r.List[1] != "static" {
		t.Errorf("slice expand: %+v", r.List)
	}
}

func TestResolve_NilInterface_NoOp(t *testing.T) {
	// feed a struct holding a nil *T — walkStrings should handle gracefully.
	type inner struct{ Name string }
	type holder struct{ Inner *inner }
	h := &holder{}
	ResolvePlaceholders(h, ResolverOpts{})
}

func TestParsePlaceholders_Edge(t *testing.T) {
	got := ParsePlaceholders("none")
	if len(got) != 0 {
		t.Fatalf("got %d", len(got))
	}
	got = ParsePlaceholders("${env:A|base64}")
	if len(got) != 1 || got[0].Filter != "base64" {
		t.Fatalf("parse filter: %+v", got)
	}
}

// --- Validator coverage ---------------------------------------------------

func TestValidate_Nil(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Fatal("want error for nil env")
	}
}

func TestValidate_InvalidStruct_MissingName(t *testing.T) {
	env := baseEnv()
	env.Metadata.Name = ""
	if err := Validate(env); err == nil {
		t.Fatal("want struct validation error")
	}
}

func TestValidate_BadCIDR(t *testing.T) {
	env := baseEnv()
	// Break the network zone CIDR.
	env.Spec.Networks.Value.Zones["public"] = NetworkZone{CIDR: "not-a-cidr"}
	if err := Validate(env); err == nil {
		t.Fatal("want cidr error")
	}
}

func TestValidate_BadIP_In_Node_IPs(t *testing.T) {
	env := baseEnv()
	n1 := env.Spec.Hypervisor.Value.Nodes["n1"]
	n1.IPs = map[string]string{"public": "not-an-ip"}
	env.Spec.Hypervisor.Value.Nodes["n1"] = n1
	if err := Validate(env); err == nil {
		t.Fatal("want ip error")
	}
}

func TestValidate_DuplicateIP_InZone(t *testing.T) {
	env := baseEnv()
	env.Spec.Hypervisor.Value.Nodes["n2"] = Node{
		Proxmox: ProxmoxRef{NodeName: "pve", VMID: 101},
		IPs:     map[string]string{"public": "10.0.0.1"},
		NICs: []NIC{
			{NameField: "net0", Usage: "public", Network: "public",
				IPv4: &IPv4Config{Address: "10.0.0.1/24"}}, // dup
		},
	}
	err := Validate(env)
	if err == nil || !strings.Contains(err.Error(), "duplicate IP") {
		t.Fatalf("want duplicate IP, got %v", err)
	}
}

func TestValidate_Cluster_NonRAC_NoInvariants(t *testing.T) {
	env := baseEnv()
	env.Spec.Cluster = &Ref[Cluster]{Value: &Cluster{Kind: "Cluster", Type: "plain"}}
	if err := Validate(env); err != nil {
		t.Fatalf("plain cluster must pass: %v", err)
	}
}

func TestValidate_RAC_Happy(t *testing.T) {
	env := baseEnv()
	n1 := env.Spec.Hypervisor.Value.Nodes["n1"]
	n1.NICs = []NIC{
		{NameField: "net0", Usage: "public", Network: "public", IPv4: &IPv4Config{Address: "10.0.0.1/24"}},
		{NameField: "net1", Usage: "private", Network: "priv", IPv4: &IPv4Config{Address: "10.1.0.1/24"}},
	}
	env.Spec.Hypervisor.Value.Nodes["n1"] = n1
	env.Spec.Networks.Value.Zones["priv"] = NetworkZone{CIDR: "10.1.0.0/24"}
	env.Spec.Cluster = &Ref[Cluster]{Value: &Cluster{
		Kind:    "Cluster",
		Type:    "oracle-rac",
		ScanIPs: []string{"10.0.0.10", "10.0.0.11", "10.0.0.12"},
	}}
	if err := Validate(env); err != nil {
		t.Fatalf("rac happy: %v", err)
	}
}

func TestValidate_RAC_MissingPublicNIC(t *testing.T) {
	env := baseEnv()
	n1 := env.Spec.Hypervisor.Value.Nodes["n1"]
	n1.NICs = []NIC{
		{NameField: "net0", Usage: "private", Network: "priv", IPv4: &IPv4Config{Address: "10.1.0.1/24"}},
	}
	env.Spec.Hypervisor.Value.Nodes["n1"] = n1
	env.Spec.Networks.Value.Zones["priv"] = NetworkZone{CIDR: "10.1.0.0/24"}
	env.Spec.Cluster = &Ref[Cluster]{Value: &Cluster{
		Kind:    "Cluster",
		Type:    "oracle-rac",
		ScanIPs: []string{"10.0.0.10", "10.0.0.11", "10.0.0.12"},
	}}
	err := Validate(env)
	if err == nil || !strings.Contains(err.Error(), "public") {
		t.Fatalf("want public NIC error, got %v", err)
	}
}

func TestValidate_DiskTag_ProperlyMatched(t *testing.T) {
	env := baseEnv()
	n1 := env.Spec.Hypervisor.Value.Nodes["n1"]
	n1.Disks[0].Tag = "u01"
	env.Spec.Hypervisor.Value.Nodes["n1"] = n1
	env.Spec.Linux = &Ref[Linux]{Value: &Linux{
		Kind: "Linux",
		Raw: map[string]any{
			"disk_layout": map[string]any{
				"additional": []any{
					map[string]any{"tag": "u01"},
					map[string]any{"not-tag": "ignored"}, // entry without tag
				},
			},
		},
	}}
	if err := Validate(env); err != nil {
		t.Fatalf("tag match: %v", err)
	}
}

func TestValidate_DiskTag_NoLayout(t *testing.T) {
	env := baseEnv()
	env.Spec.Linux = &Ref[Linux]{Value: &Linux{Kind: "Linux", Raw: map[string]any{}}}
	if err := Validate(env); err != nil {
		t.Fatalf("no layout: %v", err)
	}
}

func TestValidate_NetworkRef_EmptySkipped(t *testing.T) {
	env := baseEnv()
	n1 := env.Spec.Hypervisor.Value.Nodes["n1"]
	n1.NICs = []NIC{
		{NameField: "net0", Usage: "public"}, // no network, no IPv4 — should skip cross-field
	}
	env.Spec.Hypervisor.Value.Nodes["n1"] = n1
	if err := Validate(env); err != nil {
		t.Fatalf("empty ref skip: %v", err)
	}
}

func TestValidate_DiskStorage_EmptySkipped(t *testing.T) {
	env := baseEnv()
	n1 := env.Spec.Hypervisor.Value.Nodes["n1"]
	n1.Disks = []Disk{{ID: 0, Size: "10G"}} // no storage_class
	env.Spec.Hypervisor.Value.Nodes["n1"] = n1
	if err := Validate(env); err != nil {
		t.Fatalf("empty storage skip: %v", err)
	}
}

func TestValidate_MAC_Auto_Not_Duplicated(t *testing.T) {
	env := baseEnv()
	n1 := env.Spec.Hypervisor.Value.Nodes["n1"]
	n1.NICs[0].MAC = "auto"
	env.Spec.Hypervisor.Value.Nodes["n1"] = n1
	env.Spec.Hypervisor.Value.Nodes["n2"] = Node{
		Proxmox: ProxmoxRef{NodeName: "pve", VMID: 101},
		IPs:     map[string]string{"public": "10.0.0.2"},
		NICs: []NIC{
			{NameField: "net0", Usage: "public", Network: "public", MAC: "auto",
				IPv4: &IPv4Config{Address: "10.0.0.2/24"}},
		},
	}
	if err := Validate(env); err != nil {
		t.Fatalf("auto MAC should not clash: %v", err)
	}
}

// --- Loader + Schema + Profiles ------------------------------------------

func TestLoad_SkipSecrets(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
metadata: {name: secrets}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1:
        proxmox: {node_name: pve, vm_id: 100}
        ips: {public: 10.0.0.1}
  networks:
    kind: Networks
    public: {cidr: 10.0.0.0/24}
  storage_classes:
    kind: StorageClasses
    local: {backend: "${env:MISSING_VAR}"}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Without SkipSecrets the missing env var should fail.
	if _, err := Load(p); err == nil {
		t.Fatal("want missing env err")
	}
	// With SkipSecrets it should succeed.
	if _, err := LoadWithOptions(p, LoadOptions{SkipSecrets: true, SkipValidate: true}); err != nil {
		t.Fatalf("skip secrets: %v", err)
	}
}

func TestLoad_ParseError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	if err := os.WriteFile(p, []byte("not: [valid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want parse error")
	}
}

func TestLoad_BadProfile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
extends: nonexistent-profile
metadata: {name: x}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("want profile not found")
	}
}

func TestLoad_CircularRef(t *testing.T) {
	dir := t.TempDir()
	h := filepath.Join(dir, "h.yaml")
	e := filepath.Join(dir, "env.yaml")
	// This test exercises the seen[] path via a ref back to env.
	// Any ref pointing at env.yaml itself triggers circular detection because
	// the loader pre-seeds env.yaml into seen.
	if err := os.WriteFile(h, []byte(`kind: Hypervisor
nodes:
  n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e, []byte(`version: "1"
kind: Env
metadata: {name: circ}
spec:
  hypervisor: {$ref: ./env.yaml}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(e); err == nil || !strings.Contains(err.Error(), "circular") {
		t.Fatalf("want circular, got %v", err)
	}
}

func TestLoad_RefPathAbsolute(t *testing.T) {
	dir := t.TempDir()
	h := filepath.Join(dir, "h.yaml")
	if err := os.WriteFile(h, []byte(`kind: Hypervisor
nodes:
  n1: {proxmox: {node_name: pve, vm_id: 100}, ips: {public: 10.0.0.1}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	e := filepath.Join(dir, "env.yaml")
	// absolute path reference
	content := `version: "1"
kind: Env
metadata: {name: abspath}
spec:
  hypervisor: {$ref: ` + h + `}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	if err := os.WriteFile(e, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	env, err := Load(e)
	if err != nil {
		t.Fatalf("abs ref: %v", err)
	}
	if env.Spec.Hypervisor.Resolved() == nil {
		t.Fatal("hypervisor not resolved")
	}
}

func TestLoad_BadRefYAML(t *testing.T) {
	dir := t.TempDir()
	h := filepath.Join(dir, "h.yaml")
	if err := os.WriteFile(h, []byte("not: [unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := filepath.Join(dir, "env.yaml")
	content := `version: "1"
kind: Env
metadata: {name: bad}
spec:
  hypervisor: {$ref: ./h.yaml}
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	if err := os.WriteFile(e, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(e); err == nil {
		t.Fatal("want bad ref yaml error")
	}
}

func TestSchema_Generate(t *testing.T) {
	s, err := GenerateSchema()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "version") || !strings.Contains(s, "Env") {
		t.Errorf("schema looks empty: %q", s[:200])
	}
}

func TestLoadProfile_NotFound(t *testing.T) {
	if _, err := LoadProfile("no-such-profile"); err == nil {
		t.Fatal("want error")
	}
}

// --- YAML Unmarshal error paths -----------------------------------------

func TestNetworks_UnmarshalBadNode(t *testing.T) {
	// Scalar instead of mapping.
	var n Networks
	err := yaml.Unmarshal([]byte(`"scalar"`), &n)
	if err == nil {
		t.Fatal("want err")
	}
}

func TestNetworks_UnmarshalBadZone(t *testing.T) {
	y := `kind: Networks
public:
  cidr: 10.0.0.0/24
  gateway: [invalid]
`
	var n Networks
	if err := yaml.Unmarshal([]byte(y), &n); err == nil {
		t.Fatal("want err")
	}
}

func TestStorageClasses_UnmarshalBadNode(t *testing.T) {
	var s StorageClasses
	if err := yaml.Unmarshal([]byte(`"scalar"`), &s); err == nil {
		t.Fatal("want err")
	}
}

func TestStorageClasses_UnmarshalBadClass(t *testing.T) {
	y := `kind: StorageClasses
local:
  shared: [invalid]
`
	var s StorageClasses
	if err := yaml.Unmarshal([]byte(y), &s); err == nil {
		t.Fatal("want err")
	}
}

func TestLinux_UnmarshalBadNode(t *testing.T) {
	var l Linux
	if err := yaml.Unmarshal([]byte(`"scalar"`), &l); err == nil {
		t.Fatal("want err")
	}
}

func TestHook_UnmarshalBadNode(t *testing.T) {
	var h Hook
	if err := yaml.Unmarshal([]byte(`"scalar"`), &h); err == nil {
		t.Fatal("want err")
	}
}

func TestDatabase_UnmarshalBadNode(t *testing.T) {
	var d Database
	if err := yaml.Unmarshal([]byte(`"scalar"`), &d); err == nil {
		t.Fatal("want err")
	}
}

func TestEnv_UnmarshalBadInline(t *testing.T) {
	// Triggers inline decode error path in Ref[T].UnmarshalYAML by feeding
	// a mapping node that is not a valid Hypervisor (a list under a mapping key).
	y := `version: "1"
kind: Env
metadata: {name: e}
spec:
  hypervisor:
    kind: Hypervisor
    nodes: "not-a-map"
  networks: {kind: Networks, public: {cidr: 10.0.0.0/24}}
  storage_classes: {kind: StorageClasses, local: {backend: lvm}}
`
	var env Env
	if err := yaml.Unmarshal([]byte(y), &env); err == nil {
		t.Fatal("want inline decode error")
	}
}

func TestEnv_MarshalWithRefOnly(t *testing.T) {
	env := Env{Version: "1", Kind: "Env", Metadata: EnvMetadata{Name: "e"},
		Spec: EnvSpec{
			Hypervisor:     Ref[Hypervisor]{Ref: "./h.yaml"},
			Networks:       Ref[Networks]{Ref: "./n.yaml"},
			StorageClasses: Ref[StorageClasses]{Ref: "./s.yaml"},
		},
	}
	b, err := yaml.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "$ref") {
		t.Errorf("want $ref in output: %s", b)
	}
}

// --- mergeEnv coverage ---------------------------------------------------

func TestMergeEnv_ClusterInlineTypeFill(t *testing.T) {
	dst := &Env{Spec: EnvSpec{
		Cluster: &Ref[Cluster]{Inline: &Cluster{Kind: "Cluster"}},
	}}
	base := &Env{Version: "1", Kind: "Env",
		Metadata: EnvMetadata{Description: "base-desc"},
		Spec: EnvSpec{
			Cluster: &Ref[Cluster]{Inline: &Cluster{Kind: "Cluster", Type: "oracle-rac"}},
		},
	}
	mergeEnv(dst, base)
	if dst.Version != "1" {
		t.Errorf("version not filled: %q", dst.Version)
	}
	if dst.Kind != "Env" {
		t.Errorf("kind not filled: %q", dst.Kind)
	}
	if dst.Metadata.Description != "base-desc" {
		t.Errorf("desc not filled: %q", dst.Metadata.Description)
	}
	if dst.Spec.Cluster.Inline.Type != "oracle-rac" {
		t.Errorf("type not filled: %q", dst.Spec.Cluster.Inline.Type)
	}
}

func TestMergeEnv_DstClusterNilFilledFromBase(t *testing.T) {
	dst := &Env{}
	base := &Env{Spec: EnvSpec{Cluster: &Ref[Cluster]{Inline: &Cluster{Kind: "Cluster", Type: "plain"}}}}
	mergeEnv(dst, base)
	if dst.Spec.Cluster == nil {
		t.Fatal("cluster not copied")
	}
}

// --- profile happy path with non-empty list --------------------------------

func TestListProfiles_ContainsEmbedded(t *testing.T) {
	names, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	if !seen["pg-single"] {
		t.Errorf("want pg-single in %v", names)
	}
}
