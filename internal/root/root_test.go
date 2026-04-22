package root

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_Runs(t *testing.T) {
	cmd := New()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"vm", "workflow", "kickstart", "license", "version"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help output missing subcommand %q\nfull output:\n%s", want, out)
		}
	}
}

func TestVersionCmd_Runs(t *testing.T) {
	cmd := New()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "proxctl") {
		t.Errorf("version output missing 'proxctl': %q", buf.String())
	}
}
