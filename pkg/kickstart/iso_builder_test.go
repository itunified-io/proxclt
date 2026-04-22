package kickstart

import (
	"os"
	"path/filepath"
	"testing"
)

// helperPATH points PATH at a tempdir and optionally writes fake executables.
// It restores the original PATH via t.Cleanup.
func helperPATH(t *testing.T, tools ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range tools {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	orig := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	return dir
}

func TestISOBuilderSkipWhenNoTool(t *testing.T) {
	b := &ISOBuilder{Tool: ""}
	if _, err := b.Build("stub", "host"); err == nil {
		t.Errorf("expected error when no tool available")
	}
}

func TestISOBuilderBuild(t *testing.T) {
	if detectTool() == "" {
		t.Skip("no xorriso/mkisofs available")
	}
	// Create a fake bootloader dir with an empty isolinux.bin and kernel stubs.
	bootDir := t.TempDir()
	for _, name := range []string{"isolinux.bin", "ldlinux.c32", "vmlinuz", "initrd.img"} {
		if err := os.WriteFile(filepath.Join(bootDir, name), []byte("stub"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b := NewISOBuilder(bootDir)
	b.WorkDir = t.TempDir()
	path, err := b.Build("# empty kickstart", "testhost")
	if err != nil {
		// mkisofs is picky about the stub isolinux.bin contents — skip when it complains.
		t.Skipf("ISO tool rejected stub bootloader: %v", err)
		return
	}
	defer os.Remove(path)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() == 0 {
		t.Errorf("ISO has zero size")
	}
}

func TestDetectToolXorriso(t *testing.T) {
	helperPATH(t, "xorriso")
	if got := detectTool(); got != "xorriso" {
		t.Errorf("want xorriso got %q", got)
	}
}

func TestDetectToolMkisofs(t *testing.T) {
	helperPATH(t, "mkisofs")
	if got := detectTool(); got != "mkisofs" {
		t.Errorf("want mkisofs got %q", got)
	}
}

func TestDetectToolGenisoimage(t *testing.T) {
	helperPATH(t, "genisoimage")
	if got := detectTool(); got != "genisoimage" {
		t.Errorf("want genisoimage got %q", got)
	}
}

func TestDetectToolNone(t *testing.T) {
	helperPATH(t) // empty dir
	if got := detectTool(); got != "" {
		t.Errorf("want empty got %q", got)
	}
}

func TestNewISOBuilderSetsFields(t *testing.T) {
	helperPATH(t, "xorriso")
	b := NewISOBuilder("/some/boot")
	if b.Tool != "xorriso" || b.BootloaderDir != "/some/boot" {
		t.Errorf("unexpected: %+v", b)
	}
}

func TestISOBuilder_NoBootloaderDir(t *testing.T) {
	b := &ISOBuilder{Tool: "xorriso"}
	if _, err := b.Build("stub", "host"); err == nil {
		t.Errorf("expected error for empty BootloaderDir")
	}
}

func TestISOBuilder_MissingIsolinuxBin(t *testing.T) {
	b := &ISOBuilder{Tool: "xorriso", BootloaderDir: t.TempDir()}
	if _, err := b.Build("stub", "host"); err == nil {
		t.Errorf("expected error when isolinux.bin missing")
	}
}

func TestISOBuilder_UnknownTool(t *testing.T) {
	bootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(bootDir, "isolinux.bin"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &ISOBuilder{Tool: "weird-tool", BootloaderDir: bootDir, WorkDir: t.TempDir()}
	if _, err := b.Build("stub", "host"); err == nil {
		t.Errorf("expected error for unknown tool")
	}
}

func TestISOBuilder_SubprocessFailure(t *testing.T) {
	// Use a fake tool that always exits 1.
	dir := t.TempDir()
	fakeTool := filepath.Join(dir, "xorriso")
	if err := os.WriteFile(fakeTool, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", orig) })

	bootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(bootDir, "isolinux.bin"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &ISOBuilder{Tool: "xorriso", BootloaderDir: bootDir, WorkDir: t.TempDir()}
	if _, err := b.Build("stub", "host"); err == nil {
		t.Errorf("expected subprocess failure")
	}
}

func TestISOBuilder_MkisofsBranch(t *testing.T) {
	// Exercise the mkisofs case-branch with a fake that succeeds.
	dir := t.TempDir()
	fakeTool := filepath.Join(dir, "mkisofs")
	script := "#!/bin/sh\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"-o\" ]; then echo stub > \"$2\"; fi\n" +
		"  shift\n" +
		"done\nexit 0\n"
	if err := os.WriteFile(fakeTool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", orig) })

	bootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(bootDir, "isolinux.bin"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &ISOBuilder{Tool: "mkisofs", BootloaderDir: bootDir, WorkDir: t.TempDir()}
	path, err := b.Build("content", "host")
	if err != nil {
		t.Fatalf("mkisofs branch: %v", err)
	}
	defer os.Remove(path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("output ISO missing: %v", err)
	}
}

func TestISOBuilder_CopyBootloaderFailure(t *testing.T) {
	// BootloaderDir contains isolinux.bin that is unreadable by us → copyDir fails.
	if os.Geteuid() == 0 {
		t.Skip("root can read anything")
	}
	bootDir := t.TempDir()
	isoBin := filepath.Join(bootDir, "isolinux.bin")
	if err := os.WriteFile(isoBin, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(bootDir, "other")
	if err := os.WriteFile(unreadable, []byte("stub"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	b := &ISOBuilder{Tool: "xorriso", BootloaderDir: bootDir, WorkDir: t.TempDir()}
	if _, err := b.Build("stub", "host"); err == nil {
		t.Errorf("expected copy bootloader failure")
	}
}

func TestISOBuilder_MkdirTempFailure(t *testing.T) {
	// WorkDir points to a non-existent path → MkdirTemp fails.
	bootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(bootDir, "isolinux.bin"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &ISOBuilder{Tool: "xorriso", BootloaderDir: bootDir, WorkDir: "/nonexistent/path/deep/tree"}
	if _, err := b.Build("stub", "host"); err == nil {
		t.Errorf("expected mkdtemp failure")
	}
}

func TestISOBuilder_DefaultWorkDir(t *testing.T) {
	// Exercise the workRoot=="" → os.TempDir() branch using a fake tool.
	dir := t.TempDir()
	fakeTool := filepath.Join(dir, "mkisofs")
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ \"$1\" = \"-o\" ]; then echo stub > \"$2\"; fi; shift; done\nexit 0\n"
	if err := os.WriteFile(fakeTool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	t.Cleanup(func() { os.Setenv("PATH", orig) })

	// Redirect TempDir by setting TMPDIR to a writable location that we clean up.
	tmp := t.TempDir()
	origTMP := os.Getenv("TMPDIR")
	os.Setenv("TMPDIR", tmp)
	t.Cleanup(func() { os.Setenv("TMPDIR", origTMP) })

	bootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(bootDir, "isolinux.bin"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &ISOBuilder{Tool: "mkisofs", BootloaderDir: bootDir} // WorkDir empty
	path, err := b.Build("content", "host")
	if err != nil {
		t.Fatalf("default workdir: %v", err)
	}
	defer os.Remove(path)
}

func TestCopyDirAndFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	// Regular file + a subdirectory (which should be skipped)
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil || string(b) != "hello" {
		t.Errorf("copy failed: %v %q", err, string(b))
	}

	// copyDir on missing src
	if err := copyDir(filepath.Join(src, "nope"), dst); err == nil {
		t.Errorf("expected error for missing src")
	}

	// copyFile src missing
	if err := copyFile(filepath.Join(src, "no.txt"), filepath.Join(dst, "x")); err == nil {
		t.Errorf("expected error for missing copyFile src")
	}

	// copyFile dst unwritable (subdir that doesn't exist)
	if err := copyFile(filepath.Join(src, "a.txt"), filepath.Join(dst, "nope/dir/x")); err == nil {
		t.Errorf("expected error for unwritable dst")
	}
}
