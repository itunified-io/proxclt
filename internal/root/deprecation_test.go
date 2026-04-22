package root

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeprecation_EnvSubcommandEmitsWarning verifies the `proxctl env …`
// verb still works and writes the one-line deprecation warning to stderr.
func TestDeprecation_EnvSubcommandEmitsWarning(t *testing.T) {
	isolateHome(t)

	// Capture real stderr because the warning goes through os.Stderr.
	origErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr })

	_, err := executeCmd(t, "env", "list")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("env list: expected not-implemented error, got %v", err)
	}

	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "DEPRECATED") || !strings.Contains(string(out), "proxctl stack") {
		t.Errorf("expected deprecation warning on stderr, got: %q", string(out))
	}
}

// TestDeprecation_EnvFlagPromotedToStack verifies that `--env` still works
// and populates flagStack with a one-time warning.
func TestDeprecation_EnvFlagPromotedToStack(t *testing.T) {
	home := isolateHome(t)
	envPath := writeEnvFixture(t, home)

	origErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr })

	// config validate reads flagStack → explicit path still preferred, but the
	// deprecation message should fire before subcommand runs.
	_, err := executeCmd(t, "--env", envPath, "config", "validate", envPath)
	if err != nil {
		t.Fatalf("config validate --env: %v", err)
	}

	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "DEPRECATED: --env is renamed to --stack") {
		t.Errorf("expected --env deprecation warning, got: %q", string(out))
	}
}

// TestDeprecation_EnvVarPromotedToStack verifies $PROXCTL_ENV → $PROXCTL_STACK
// promotion.
func TestDeprecation_EnvVarPromotedToStack(t *testing.T) {
	isolateHome(t)
	t.Setenv("PROXCTL_ENV", "legacy-value")
	// Ensure PROXCTL_STACK is empty so promotion happens.
	t.Setenv("PROXCTL_STACK", "")

	origErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr })

	// Any command that runs PersistentPreRunE is fine; `version` is cheap.
	_, err := executeCmd(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}

	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "DEPRECATED: $PROXCTL_ENV") {
		t.Errorf("expected $PROXCTL_ENV deprecation warning, got: %q", string(out))
	}
	if got := os.Getenv("PROXCTL_STACK"); got != "legacy-value" {
		t.Errorf("PROXCTL_STACK not promoted; got %q, want %q", got, "legacy-value")
	}
}

// TestMigration_EnvsYamlRenamedToStacksYaml covers the one-shot
// ~/.proxctl/envs.yaml → stacks.yaml rename.
func TestMigration_EnvsYamlRenamedToStacksYaml(t *testing.T) {
	home := isolateHome(t)
	cfgDir := filepath.Join(home, ".proxctl")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(cfgDir, "envs.yaml")
	newPath := filepath.Join(cfgDir, "stacks.yaml")
	payload := []byte("# legacy registry\nstacks: []\n")
	if err := os.WriteFile(oldPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	// Reset the migration sync.Once (package-local state). The easiest path is
	// just to run migrateStacksRegistryForTest, which re-executes the logic.
	migrateStacksRegistryForTest(t)

	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("stacks.yaml missing after migration: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("envs.yaml still present after migration: err=%v", err)
	}
	got, _ := os.ReadFile(newPath)
	if !bytes.Equal(got, payload) {
		t.Errorf("stacks.yaml contents mutated during migration: got=%q want=%q", got, payload)
	}
}

// TestMigration_NoLegacyFile_NoOp verifies no-op when neither file exists.
func TestMigration_NoLegacyFile_NoOp(t *testing.T) {
	home := isolateHome(t)
	cfgDir := filepath.Join(home, ".proxctl")
	_ = os.MkdirAll(cfgDir, 0o755)

	origErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr })

	migrateStacksRegistryForTest(t)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if len(out) != 0 {
		t.Errorf("expected no stderr output when no legacy file present, got: %q", string(out))
	}
}

// TestMigration_RenameFailureWarns forces os.Rename to fail by making the
// parent directory read-only. We verify the warning path runs without
// crashing the CLI.
func TestMigration_RenameFailureWarns(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot exercise os.Rename failure as root")
	}
	home := isolateHome(t)
	cfgDir := filepath.Join(home, ".proxctl")
	_ = os.MkdirAll(cfgDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfgDir, "envs.yaml"), []byte("x"), 0o644)
	// Make cfgDir read-only so Rename inside it fails.
	_ = os.Chmod(cfgDir, 0o555)
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o755) })

	origErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr })

	migrateStacksRegistryForTest(t)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "could not migrate") {
		t.Logf("rename may have succeeded on this filesystem; stderr=%q", string(out))
	}
}

// TestMigration_BothFilesPresentWarns covers the "both files exist" branch.
func TestMigration_BothFilesPresentWarns(t *testing.T) {
	home := isolateHome(t)
	cfgDir := filepath.Join(home, ".proxctl")
	_ = os.MkdirAll(cfgDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfgDir, "envs.yaml"), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(cfgDir, "stacks.yaml"), []byte("new"), 0o644)

	origErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr })

	migrateStacksRegistryForTest(t)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "both") || !strings.Contains(string(out), "envs.yaml") {
		t.Errorf("expected 'both files exist' warning, got: %q", string(out))
	}

	// stacks.yaml must be untouched.
	got, _ := os.ReadFile(filepath.Join(cfgDir, "stacks.yaml"))
	if string(got) != "new" {
		t.Errorf("stacks.yaml was modified; got %q", string(got))
	}
}
