package config

import (
	"strings"
	"testing"
)

type rootStruct struct {
	Name   string
	Secret string
	Extra  map[string]string
	List   []string
}

func TestResolve_Env(t *testing.T) {
	r := &rootStruct{Secret: "${env:MY_TOK}"}
	opts := ResolverOpts{LookupEnv: func(k string) (string, bool) {
		if k == "MY_TOK" {
			return "s3cret", true
		}
		return "", false
	}}
	res := ResolvePlaceholders(r, opts)
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if r.Secret != "s3cret" {
		t.Errorf("got %q", r.Secret)
	}
}

func TestResolve_EnvMissing(t *testing.T) {
	r := &rootStruct{Secret: "${env:UNSET_VAR}"}
	res := ResolvePlaceholders(r, ResolverOpts{LookupEnv: func(string) (string, bool) { return "", false }})
	if len(res.Errors) == 0 {
		t.Fatal("want error for missing env var")
	}
}

func TestResolve_EnvWithDefault(t *testing.T) {
	r := &rootStruct{Secret: "${env:UNSET_VAR|default:fallback}"}
	res := ResolvePlaceholders(r, ResolverOpts{LookupEnv: func(string) (string, bool) { return "", false }})
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if r.Secret != "fallback" {
		t.Errorf("got %q", r.Secret)
	}
}

func TestResolve_File(t *testing.T) {
	r := &rootStruct{Secret: "${file:/etc/hostname}"}
	opts := ResolverOpts{ReadFile: func(p string) ([]byte, error) {
		return []byte("the-host\n"), nil
	}}
	res := ResolvePlaceholders(r, opts)
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if r.Secret != "the-host" {
		t.Errorf("got %q", r.Secret)
	}
}

func TestResolve_Vault_Deferred(t *testing.T) {
	r := &rootStruct{Secret: "${vault:secret/data/foo#bar}"}
	res := ResolvePlaceholders(r, ResolverOpts{})
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if !strings.HasPrefix(r.Secret, "<VAULT:") {
		t.Errorf("want deferred marker, got %q", r.Secret)
	}
	if len(res.Unresolved) != 1 {
		t.Errorf("unresolved count: %v", res.Unresolved)
	}
}

func TestResolve_GenPassword_Deterministic(t *testing.T) {
	r1 := &rootStruct{Secret: "${gen:password:16}"}
	r2 := &rootStruct{Secret: "${gen:password:16}"}
	ResolvePlaceholders(r1, ResolverOpts{EnvName: "same"})
	ResolvePlaceholders(r2, ResolverOpts{EnvName: "same"})
	if r1.Secret != r2.Secret {
		t.Errorf("not deterministic: %q vs %q", r1.Secret, r2.Secret)
	}
	if len(r1.Secret) != 16 {
		t.Errorf("len: %d", len(r1.Secret))
	}
}

func TestResolve_Base64Filter(t *testing.T) {
	r := &rootStruct{Secret: "${env:MY|base64}"}
	res := ResolvePlaceholders(r, ResolverOpts{LookupEnv: func(k string) (string, bool) {
		return "abc", true
	}})
	if len(res.Errors) != 0 {
		t.Fatalf("%v", res.Errors)
	}
	if r.Secret != "YWJj" {
		t.Errorf("got %q", r.Secret)
	}
}

func TestResolve_Ref(t *testing.T) {
	env := &Env{
		Metadata: EnvMetadata{Name: "my-env"},
	}
	env.Metadata.Tags = map[string]string{"zone": "eu", "name": "${ref:Metadata.Name}"}
	ResolvePlaceholders(env, ResolverOpts{})
	if env.Metadata.Tags["name"] != "my-env" {
		t.Errorf("ref not expanded: %q", env.Metadata.Tags["name"])
	}
}

func TestParsePlaceholders(t *testing.T) {
	in := "a=${env:X} b=${file:/tmp/y} c=${vault:path#key} d=${gen:password:12}"
	got := ParsePlaceholders(in)
	if len(got) != 4 {
		t.Fatalf("count: %d", len(got))
	}
}
