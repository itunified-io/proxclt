package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLinux_Roundtrip(t *testing.T) {
	y := `
kind: Linux
disk_layout:
  additional:
    - tag: u01
      mount: /u01
users:
  oracle: {uid: 1101}
`
	var l Linux
	if err := yaml.Unmarshal([]byte(y), &l); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if l.Kind != "Linux" {
		t.Errorf("kind: %q", l.Kind)
	}
	if _, ok := l.Raw["disk_layout"]; !ok {
		t.Errorf("missing disk_layout in raw: %+v", l.Raw)
	}
	out, err := yaml.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "kind: Linux") {
		t.Errorf("marshal missing kind: %s", out)
	}
	if !strings.Contains(string(out), "disk_layout") {
		t.Errorf("marshal missing disk_layout: %s", out)
	}
}

func TestDatabase_Roundtrip(t *testing.T) {
	y := `
kind: OracleDatabase
name: CDBTEST
version: "19.25"
`
	var d Database
	if err := yaml.Unmarshal([]byte(y), &d); err != nil {
		t.Fatal(err)
	}
	if d.Kind != "OracleDatabase" {
		t.Errorf("kind: %q", d.Kind)
	}
	if d.Raw["name"] != "CDBTEST" {
		t.Errorf("name: %v", d.Raw["name"])
	}
	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "OracleDatabase") {
		t.Errorf("marshal: %s", out)
	}
}

func TestHooks_Roundtrip(t *testing.T) {
	y := `
on_apply_success:
  - type: slack
    channel: "#ops"
    message: done
  - type: exec
    cmd: echo ok
`
	var h Hooks
	if err := yaml.Unmarshal([]byte(y), &h); err != nil {
		t.Fatal(err)
	}
	if len(h.OnApplySuccess) != 2 {
		t.Fatalf("hooks count: %d", len(h.OnApplySuccess))
	}
	if h.OnApplySuccess[0].Type != "slack" {
		t.Errorf("type: %q", h.OnApplySuccess[0].Type)
	}
	if h.OnApplySuccess[0].Params["channel"] != "#ops" {
		t.Errorf("params: %+v", h.OnApplySuccess[0].Params)
	}
	out, err := yaml.Marshal(h.OnApplySuccess[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "slack") {
		t.Errorf("marshal hook: %s", out)
	}
}

func TestResolve_GenSSHKey(t *testing.T) {
	r := &rootStruct{Secret: "${gen:ssh_key:ed25519}"}
	res := ResolvePlaceholders(r, ResolverOpts{})
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if !strings.HasPrefix(r.Secret, "<GEN_SSH_KEY:") {
		t.Errorf("got %q", r.Secret)
	}
	if len(res.Unresolved) != 1 {
		t.Errorf("unresolved: %v", res.Unresolved)
	}
}

func TestWalkStringsForTest(t *testing.T) {
	r := &rootStruct{Name: "hello"}
	WalkStringsForTest(r, func(s string) string { return s + "!" })
	if r.Name != "hello!" {
		t.Errorf("got %q", r.Name)
	}
}

func TestRef_Resolved(t *testing.T) {
	r := &Ref[Cluster]{Inline: &Cluster{Kind: "Cluster", Type: "plain"}}
	if r.Resolved() == nil {
		t.Fatal("inline should resolve")
	}
	r2 := &Ref[Cluster]{Value: &Cluster{Kind: "Cluster", Type: "oracle-rac"}}
	if r2.Resolved().Type != "oracle-rac" {
		t.Errorf("value should win")
	}
	var r3 *Ref[Cluster]
	if r3.Resolved() != nil {
		t.Error("nil should return nil")
	}
}
