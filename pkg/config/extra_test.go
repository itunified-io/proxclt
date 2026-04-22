package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRef_MarshalRef(t *testing.T) {
	r := Ref[Cluster]{Ref: "./c.yaml"}
	b, err := yaml.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "$ref") {
		t.Errorf("marshal ref: %s", b)
	}
}

func TestRef_MarshalEmpty(t *testing.T) {
	var r Ref[Cluster]
	b, err := yaml.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return
	}
}

func TestLoad_LinuxAndDatabases(t *testing.T) {
	dir := t.TempDir()
	writeFile := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	writeFile("linux.yaml", `kind: Linux
users:
  oracle: {uid: 1101}
`)
	writeFile("db.yaml", `kind: OracleDatabase
name: CDB1
`)
	envPath := writeFile("env.yaml", `version: "1"
kind: Env
metadata: {name: full}
spec:
  hypervisor:
    kind: Hypervisor
    nodes:
      n1:
        proxmox: {node_name: pve, vm_id: 100}
        ips: {public: 10.0.0.1}
  linux:
    $ref: ./linux.yaml
  networks:
    kind: Networks
    public: {cidr: 10.0.0.0/24}
  storage_classes:
    kind: StorageClasses
    local: {backend: lvm}
  databases:
    - $ref: ./db.yaml
`)
	env, err := Load(envPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if env.Spec.Linux == nil || env.Spec.Linux.Resolved() == nil {
		t.Fatal("linux not resolved")
	}
	if len(env.Spec.Databases) != 1 {
		t.Fatalf("databases count: %d", len(env.Spec.Databases))
	}
	if env.Spec.Databases[0].Resolved().Kind != "OracleDatabase" {
		t.Errorf("db kind: %q", env.Spec.Databases[0].Resolved().Kind)
	}
}

func TestResolve_MapRecursion(t *testing.T) {
	env := &Env{
		Metadata: EnvMetadata{
			Name: "e",
			Tags: map[string]string{"k": "${env:MY}"},
		},
	}
	ResolvePlaceholders(env, ResolverOpts{LookupEnv: func(string) (string, bool) { return "v", true }})
	if env.Metadata.Tags["k"] != "v" {
		t.Errorf("map string not expanded: %q", env.Metadata.Tags["k"])
	}
}

func TestExpandHome(t *testing.T) {
	p := expandHome("~/foo")
	if !strings.HasSuffix(p, "/foo") || strings.HasPrefix(p, "~/") {
		t.Errorf("expandHome: %q", p)
	}
	if got := expandHome("/abs/path"); got != "/abs/path" {
		t.Errorf("abs unchanged: %q", got)
	}
}
