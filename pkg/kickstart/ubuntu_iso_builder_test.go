package kickstart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchKernelCmdline_GrubLinuxLine(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "grub-orig.cfg")
	dst := filepath.Join(dir, "grub-new.cfg")
	in := `set timeout=30
menuentry "Try or Install Ubuntu Server" {
    set gfxpayload=keep
    linux   /casper/vmlinuz   ---
    initrd  /casper/initrd
}`
	if err := os.WriteFile(src, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := patchKernelCmdline(src, dst, `autoinstall ds=nocloud\;s=/cdrom/cidata/`); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !strings.Contains(string(got), `autoinstall ds=nocloud\;s=/cdrom/cidata/`) {
		t.Errorf("autoinstall args not injected:\n%s", got)
	}
	if !strings.Contains(string(got), `/casper/vmlinuz`) || !strings.Contains(string(got), `---`) {
		t.Errorf("original linux line content lost:\n%s", got)
	}
}

func TestPatchKernelCmdline_Idempotent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.cfg")
	dst := filepath.Join(dir, "dst.cfg")
	already := "linux /casper/vmlinuz autoinstall ds=nocloud\\;s=/cdrom/cidata/ ---\n"
	os.WriteFile(src, []byte(already), 0o644)
	patchKernelCmdline(src, dst, `autoinstall ds=nocloud\;s=/cdrom/cidata/`)
	got, _ := os.ReadFile(dst)
	// Should NOT have appended a second copy of autoinstall.
	if strings.Count(string(got), "autoinstall") != 1 {
		t.Errorf("idempotency broken — autoinstall appears %d times:\n%s",
			strings.Count(string(got), "autoinstall"), got)
	}
}

func TestPatchKernelCmdline_IsolinuxAppend(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "txt.cfg")
	dst := filepath.Join(dir, "txt-new.cfg")
	in := `default install
label install
  menu label ^Install Ubuntu Server
  kernel /casper/vmlinuz
  append initrd=/casper/initrd quiet ---
`
	os.WriteFile(src, []byte(in), 0o644)
	patchKernelCmdline(src, dst, `autoinstall ds=nocloud\;s=/cdrom/cidata/`)
	got, _ := os.ReadFile(dst)
	if !strings.Contains(string(got), `autoinstall ds=nocloud\;s=/cdrom/cidata/`) {
		t.Errorf("autoinstall not appended to isolinux append line:\n%s", got)
	}
	// kernel line untouched (we only patch append/linux/linuxefi).
	if !strings.Contains(string(got), "kernel /casper/vmlinuz\n") {
		t.Errorf("kernel line should be untouched:\n%s", got)
	}
}

func TestPatchKernelCmdline_InjectsBeforeCasperSeparator(t *testing.T) {
	// Critical: in casper boot, args AFTER `---` are reserved for the
	// post-install kernel and NOT seen by Subiquity in the live boot.
	// `autoinstall` MUST appear BEFORE `---`.
	dir := t.TempDir()
	src := filepath.Join(dir, "grub.cfg")
	dst := filepath.Join(dir, "grub-new.cfg")
	in := "menuentry \"Install\" {\n    linux /casper/vmlinuz quiet --- foo bar\n}\n"
	os.WriteFile(src, []byte(in), 0o644)
	patchKernelCmdline(src, dst, `autoinstall ds=nocloud\;s=/cdrom/cidata/`)
	got, _ := os.ReadFile(dst)
	gotS := string(got)
	autoIdx := strings.Index(gotS, "autoinstall")
	sepIdx := strings.Index(gotS, "---")
	if autoIdx < 0 || sepIdx < 0 || autoIdx > sepIdx {
		t.Errorf("autoinstall must appear BEFORE casper `---` separator:\n%s", gotS)
	}
}

func TestPatchKernelCmdline_LinuxefiVariant(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "grub.cfg")
	dst := filepath.Join(dir, "grub-new.cfg")
	in := `menuentry "Install" {
    linuxefi /casper/vmlinuz quiet
    initrdefi /casper/initrd
}`
	os.WriteFile(src, []byte(in), 0o644)
	patchKernelCmdline(src, dst, `autoinstall ds=nocloud\;s=/cdrom/cidata/`)
	got, _ := os.ReadFile(dst)
	if !strings.Contains(string(got), `linuxefi /casper/vmlinuz quiet autoinstall ds=nocloud\;s=/cdrom/cidata/`) {
		t.Errorf("linuxefi variant not patched:\n%s", got)
	}
}

func TestNewUbuntuISOBuilder_Defaults(t *testing.T) {
	b := NewUbuntuISOBuilder("/tmp/foo.iso", "/tmp/cidata")
	if b.AutoinstallKernelArgs != `autoinstall ds=nocloud\;s=/cdrom/cidata/` {
		t.Errorf("default autoinstall args: %q", b.AutoinstallKernelArgs)
	}
}

func TestUbuntuISOBuilder_RejectsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	cidata := filepath.Join(dir, "cidata")
	os.MkdirAll(cidata, 0o755)

	b := &UbuntuISOBuilder{
		SourceISOPath: "/nonexistent/foo.iso",
		CIDataDir:     cidata,
	}
	if _, err := b.Build("test"); err == nil {
		t.Error("expected error for missing source ISO")
	}

	// cidata missing user-data
	iso := filepath.Join(dir, "fake.iso")
	os.WriteFile(iso, []byte("fake"), 0o644)
	b.SourceISOPath = iso
	if _, err := b.Build("test"); err == nil {
		t.Error("expected error for missing user-data")
	}
}
