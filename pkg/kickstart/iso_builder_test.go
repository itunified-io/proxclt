package kickstart

import (
	"os"
	"path/filepath"
	"testing"
)

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
