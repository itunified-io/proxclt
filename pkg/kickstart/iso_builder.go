package kickstart

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// ISOBuilder remasters a minimal bootable ISO that embeds a kickstart at /ks.cfg.
type ISOBuilder struct {
	// Tool is "xorriso" (preferred) or "mkisofs" (fallback). Auto-detected when empty.
	Tool string
	// BootloaderDir is a directory containing isolinux.bin, ldlinux.c32, vmlinuz, initrd.img.
	BootloaderDir string
	// WorkDir is the temp dir root. Defaults to os.TempDir() when empty.
	WorkDir string
}

// NewISOBuilder creates a builder auto-detecting the ISO tool.
func NewISOBuilder(bootloaderDir string) *ISOBuilder {
	return &ISOBuilder{
		Tool:          detectTool(),
		BootloaderDir: bootloaderDir,
	}
}

// detectTool picks xorriso if available, else mkisofs, else empty.
func detectTool() string {
	if _, err := exec.LookPath("xorriso"); err == nil {
		return "xorriso"
	}
	if _, err := exec.LookPath("mkisofs"); err == nil {
		return "mkisofs"
	}
	if _, err := exec.LookPath("genisoimage"); err == nil {
		return "genisoimage"
	}
	return ""
}

// Build writes a bootable ISO containing /ks.cfg and returns the ISO path.
// The caller is responsible for deleting the returned file when done.
func (b *ISOBuilder) Build(kickstartContent, hostname string) (string, error) {
	if b.Tool == "" {
		return "", errors.New("no ISO tool available (install xorriso or mkisofs)")
	}
	if b.BootloaderDir == "" {
		return "", errors.New("BootloaderDir is required")
	}
	// Sanity: bootloader dir must contain isolinux.bin.
	if _, err := os.Stat(filepath.Join(b.BootloaderDir, "isolinux.bin")); err != nil {
		return "", fmt.Errorf("bootloader dir missing isolinux.bin: %w", err)
	}

	workRoot := b.WorkDir
	if workRoot == "" {
		workRoot = os.TempDir()
	}
	buildDir, err := os.MkdirTemp(workRoot, "proxclt-iso-"+hostname+"-")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}
	defer os.RemoveAll(buildDir)

	// Copy bootloader files into buildDir.
	if err := copyDir(b.BootloaderDir, buildDir); err != nil {
		return "", fmt.Errorf("copy bootloader: %w", err)
	}

	// Write the kickstart.
	ksPath := filepath.Join(buildDir, "ks.cfg")
	if err := os.WriteFile(ksPath, []byte(kickstartContent), 0o644); err != nil {
		return "", fmt.Errorf("write ks.cfg: %w", err)
	}

	// Write isolinux.cfg.
	isolinuxCfg := `DEFAULT linux
PROMPT 0
TIMEOUT 10
LABEL linux
  KERNEL vmlinuz
  APPEND initrd=initrd.img inst.ks=cdrom:/ks.cfg inst.text
`
	if err := os.WriteFile(filepath.Join(buildDir, "isolinux.cfg"), []byte(isolinuxCfg), 0o644); err != nil {
		return "", fmt.Errorf("write isolinux.cfg: %w", err)
	}

	// Build the ISO.
	outPath := filepath.Join(workRoot, fmt.Sprintf("proxclt-%s.iso", hostname))
	// Remove any stale output.
	_ = os.Remove(outPath)

	var cmd *exec.Cmd
	switch b.Tool {
	case "xorriso":
		cmd = exec.Command("xorriso", "-as", "mkisofs",
			"-o", outPath,
			"-b", "isolinux.bin",
			"-c", "boot.cat",
			"-no-emul-boot",
			"-boot-load-size", "4",
			"-boot-info-table",
			"-V", "Kickstart",
			"-J", "-r",
			buildDir,
		)
	case "mkisofs", "genisoimage":
		cmd = exec.Command(b.Tool,
			"-o", outPath,
			"-b", "isolinux.bin",
			"-c", "boot.cat",
			"-no-emul-boot",
			"-boot-load-size", "4",
			"-boot-info-table",
			"-V", "Kickstart",
			"-J", "-r",
			buildDir,
		)
	default:
		return "", fmt.Errorf("unknown tool %q", b.Tool)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s failed: %w: %s", b.Tool, err, string(out))
	}
	return outPath, nil
}

// copyDir copies regular files from src into dst (non-recursive; we assume flat bootloader dir).
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// Preserve mode (helpful for isolinux.bin).
	if fi, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, fi.Mode())
	}
	return nil
}
