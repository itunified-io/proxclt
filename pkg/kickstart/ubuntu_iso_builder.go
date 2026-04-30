package kickstart

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// UbuntuISOBuilder remasters a Ubuntu 22.04+ live-server ISO to enable
// fully unattended Subiquity autoinstall.
//
// Why a separate builder:
//
// Ubuntu 22.04.5+ live-server ISOs ship Subiquity (autoinstall via
// cloud-init datasource) — no preseed, no isolinux. The legacy
// ISOBuilder which builds a small bootable kickstart-only ISO doesn't
// apply. Instead, autoinstall requires:
//
//  1. The install ISO itself, modified so its GRUB linux entries pass
//     `autoinstall ds=nocloud\;s=/cdrom/cidata/` on the kernel cmdline. Without
//     this, Subiquity prompts the operator to confirm — defeating the
//     purpose of automation.
//  2. A `cidata` directory containing user-data + meta-data added to the
//     ISO root. Subiquity reads it from the kernel-cmdline-specified path.
//
// Implementation:
//
//   - Use xorriso `-indev`/`-outdev` mode to repack without 3 GB of
//     extraction overhead. xorriso preserves the original El Torito
//     boot image (BIOS) + UEFI eltorito.img automatically with
//     `-boot_image any replay`.
//   - Inject `autoinstall ds=nocloud\;s=/cdrom/cidata/` into every `linux`
//     line of /boot/grub/grub.cfg + isolinux/txt.cfg if present.
//   - Add the cidata files at /cidata/ so Subiquity finds them.
//
// Operator can override the install ISO via the env manifest's
// `iso.image`; the original is read-only — we always write a new
// ISO file alongside it.
type UbuntuISOBuilder struct {
	// SourceISOPath is the path to the unmodified Ubuntu install ISO.
	SourceISOPath string
	// CIDataDir is a directory containing user-data and meta-data files.
	// Both must be non-empty cloud-init-format files.
	CIDataDir string
	// WorkDir for extraction + ISO repack scratch space.
	WorkDir string
	// AutoinstallKernelArgs is appended to GRUB linux lines.
	// Default: "autoinstall ds=nocloud\\;s=/cidata/"
	AutoinstallKernelArgs string
}

// NewUbuntuISOBuilder is the canonical constructor.
func NewUbuntuISOBuilder(sourceISOPath, cidataDir string) *UbuntuISOBuilder {
	return &UbuntuISOBuilder{
		SourceISOPath:         sourceISOPath,
		CIDataDir:             cidataDir,
		AutoinstallKernelArgs: `autoinstall ds=nocloud\;s=/cdrom/cidata/`,
	}
}

// Build produces a remastered ISO with autoinstall enabled.
// The returned path is in WorkDir (or os.TempDir if WorkDir is empty).
// Caller is responsible for deleting the result + the source CIDataDir
// after use.
func (b *UbuntuISOBuilder) Build(hostname string) (string, error) {
	if b.SourceISOPath == "" {
		return "", fmt.Errorf("UbuntuISOBuilder: SourceISOPath is required")
	}
	if b.CIDataDir == "" {
		return "", fmt.Errorf("UbuntuISOBuilder: CIDataDir is required")
	}
	if _, err := os.Stat(b.SourceISOPath); err != nil {
		return "", fmt.Errorf("source ISO not found: %w", err)
	}
	for _, name := range []string{"user-data", "meta-data"} {
		if _, err := os.Stat(filepath.Join(b.CIDataDir, name)); err != nil {
			return "", fmt.Errorf("cidata dir missing %s: %w", name, err)
		}
	}
	if _, err := exec.LookPath("xorriso"); err != nil {
		return "", fmt.Errorf("xorriso not on PATH (Ubuntu ISO repack requires xorriso)")
	}

	workRoot := b.WorkDir
	if workRoot == "" {
		workRoot = os.TempDir()
	}
	scratch, err := os.MkdirTemp(workRoot, "proxctl-ubuntu-"+hostname+"-")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}
	// Caller can defer-clean. We do NOT remove on success because the
	// returned ISO lives in this dir.

	// Extract grub.cfg + isolinux/txt.cfg (if present) so we can patch.
	for _, p := range []string{"boot/grub/grub.cfg", "isolinux/txt.cfg"} {
		if err := extractISOFile(b.SourceISOPath, p, filepath.Join(scratch, "orig-"+filepath.Base(p))); err != nil {
			// Some ISOs have only grub (UEFI-only); txt.cfg may be absent.
			// grub.cfg MUST exist for any modern Ubuntu live-server.
			if p == "boot/grub/grub.cfg" {
				return "", fmt.Errorf("extract %s: %w", p, err)
			}
		}
	}

	// Patch grub.cfg.
	grubOrig := filepath.Join(scratch, "orig-grub.cfg")
	grubNew := filepath.Join(scratch, "grub.cfg")
	if err := patchKernelCmdline(grubOrig, grubNew, b.AutoinstallKernelArgs); err != nil {
		return "", fmt.Errorf("patch grub.cfg: %w", err)
	}

	// Patch isolinux/txt.cfg if it exists.
	txtOrig := filepath.Join(scratch, "orig-txt.cfg")
	txtNew := filepath.Join(scratch, "txt.cfg")
	hasIsolinux := false
	if _, err := os.Stat(txtOrig); err == nil {
		if err := patchKernelCmdline(txtOrig, txtNew, b.AutoinstallKernelArgs); err != nil {
			return "", fmt.Errorf("patch isolinux/txt.cfg: %w", err)
		}
		hasIsolinux = true
	}

	// Build the remastered ISO via xorriso indev/outdev.
	outPath := filepath.Join(scratch, fmt.Sprintf("%s_install_autoinstall.iso", hostname))
	args := []string{
		"-indev", b.SourceISOPath,
		"-outdev", outPath,
		"-boot_image", "any", "replay",
		// Patch grub.cfg
		"-map", grubNew, "/boot/grub/grub.cfg",
		// Embed cidata files at /cidata/
		"-map", filepath.Join(b.CIDataDir, "user-data"), "/cidata/user-data",
		"-map", filepath.Join(b.CIDataDir, "meta-data"), "/cidata/meta-data",
		// Joliet + Rock Ridge for cross-platform readability
		"-joliet", "on",
		"-rockridge", "on",
	}
	if hasIsolinux {
		args = append(args[:len(args)-4],
			append([]string{"-map", txtNew, "/isolinux/txt.cfg"}, args[len(args)-4:]...)...)
	}

	cmd := exec.Command("xorriso", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("xorriso repack failed: %w: %s", err, string(out))
	}
	return outPath, nil
}

// extractISOFile copies a single file from inside an ISO to dstPath
// using xorriso. Used to read grub.cfg and txt.cfg for patching.
func extractISOFile(isoPath, srcPath, dstPath string) error {
	tmpDir, err := os.MkdirTemp("", "proxctl-extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("xorriso", "-osirrox", "on", "-indev", isoPath,
		"-extract", "/"+srcPath, filepath.Join(tmpDir, filepath.Base(srcPath)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xorriso extract: %w: %s", err, string(out))
	}
	in, err := os.Open(filepath.Join(tmpDir, filepath.Base(srcPath)))
	if err != nil {
		return fmt.Errorf("read extracted: %w", err)
	}
	defer in.Close()
	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// patchKernelCmdline rewrites lines beginning with `linux` (or `kernel`/`append`
// for legacy isolinux) to inject the supplied autoinstall args, unless they're
// already present. Idempotent: re-running on an already-patched config is a
// no-op.
//
// Critical detail: in Ubuntu casper boot, `---` on the linux line separates
// arguments seen by the live boot kernel (BEFORE `---`) from arguments
// reserved for the post-install kernel (AFTER `---`). Subiquity reads
// `/proc/cmdline` of the live boot, so `autoinstall` MUST appear BEFORE
// `---` to take effect — otherwise Subiquity defaults to interactive mode
// and prompts the operator to confirm. We insert immediately before the
// `---` separator when present, else append.
//
// Match patterns:
//   - GRUB:     `linux ... [---] <args>`
//   - isolinux: `append ... [---] <args>`
func patchKernelCmdline(srcPath, dstPath, autoinstallArgs string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	// Match the leading verb (linux | linuxefi | append) at the start of a
	// trimmed line. Preserve original indentation via a capture group.
	re := regexp.MustCompile(`^(\s*)(linux|linuxefi|append)\s+(.*)$`)
	for i, line := range lines {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		body := m[3]
		if strings.Contains(body, "autoinstall") {
			// Already patched.
			continue
		}
		newBody := injectBeforeSeparator(body, autoinstallArgs)
		lines[i] = m[1] + m[2] + " " + newBody
	}
	return os.WriteFile(dstPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// injectBeforeSeparator inserts args immediately before the casper `---`
// separator if present; otherwise appends to the end.
func injectBeforeSeparator(body, args string) string {
	// Look for `---` as a standalone token (surrounded by whitespace, or at
	// end of line). Use a regex anchored on word boundaries via spaces.
	sepRE := regexp.MustCompile(`(\s)---(\s|$)`)
	if loc := sepRE.FindStringIndex(body); loc != nil {
		// Insert args before the matched `(\s)---`. loc[0] is the leading
		// whitespace; insert just after it but before the `---`.
		return body[:loc[0]] + " " + args + body[loc[0]:]
	}
	return body + " " + args
}
