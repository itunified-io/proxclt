# Distro guide

How proxctl renders unattended-install configs per Linux distribution, and how
to add a new distro.

- [Supported distros](#supported-distros)
- [DistroProfile interface](#distroprofile-interface)
- [Adding a new distro](#adding-a-new-distro)
- [Bootloader requirements](#bootloader-requirements)
- [Package set differences](#package-set-differences)

---

## Supported distros

| Distro            | `kickstart.distro` value | Status at launch | Unattended format |
|-------------------|--------------------------|------------------|-------------------|
| Oracle Linux 8    | `oraclelinux8`           | supported        | kickstart (Anaconda) |
| Oracle Linux 9    | `oraclelinux9`           | supported        | kickstart (Anaconda) |
| Ubuntu 22.04 LTS  | `ubuntu2204`             | supported        | autoinstall (subiquity) |
| RHEL 9            | `rhel9`                  | drop-in          | kickstart (Anaconda) |
| Rocky Linux 9     | `rocky9`                 | drop-in          | kickstart (Anaconda) |
| SLES 15           | `sles15`                 | drop-in          | AutoYaST |

"Drop-in" means the code path exists and the validator accepts the value; the
template is a thin specialisation of the matching RHEL-family template and
has been smoke-tested but is not part of the CI gating suite.

List the supported distros from the CLI:

```bash
proxctl kickstart distros
# oraclelinux8
# oraclelinux9
# ubuntu2204
# rhel9
# rocky9
# sles15
```

---

## DistroProfile interface

Under the hood each distro is represented by a Go struct satisfying the
`DistroProfile` interface in
[`pkg/kickstart`](../pkg/kickstart):

```go
type DistroProfile interface {
    Name() string                        // "oraclelinux9"
    Family() string                      // "rhel" | "debian" | "suse"
    DefaultImage() string                // canonical ISO filename hint
    BootloaderFiles() []string           // files copied to the first-boot ISO
    TemplateFS() fs.FS                   // embedded template directory
    KickstartFilename() string           // "ks.cfg" / "user-data"
    VolIDHint() string                   // ISO volume label
}
```

The renderer is a plain `text/template` + sprig pipeline. Templates live
under `pkg/kickstart/templates/<distro>/` and import a `common/` partial for
shared bits (chrony config, sshd hardening, sudo stanza).

Rendering flow:

1. Pick the `DistroProfile` matching `kickstart.distro`.
2. Feed the `KickstartConfig` (from `hypervisor.yaml`) into
   `DistroProfile.TemplateFS()`'s entrypoint template.
3. Write the rendered output to the first-boot ISO at
   `DistroProfile.KickstartFilename()`.
4. Build the ISO with `xorriso` (or `mkisofs`) using the distro's bootloader
   files + volume id.

---

## Adding a new distro

Checklist:

1. **Pick `kickstart.distro` value.** Lowercase, no punctuation
   (`almalinux9`, `debian12`).
2. **Add to the validator.** `pkg/config/hypervisor.go` — append to the
   `oneof` list on `KickstartConfig.Distro`.
3. **Create the template directory.**
   ```
   pkg/kickstart/templates/almalinux9/
   ├── ks.cfg.tmpl          # entrypoint
   └── partials/
       └── packages.tmpl
   ```
   Import `common/*` partials via `{{ template "common/chrony" . }}` etc.
4. **Implement the `DistroProfile`.** Typically a struct embedding the
   relevant family base (e.g. `RHELFamily`) and overriding
   `Name()` / `DefaultImage()`.
5. **Register it.** Append to the slice returned by
   `pkg/kickstart/profiles.go:AllProfiles()`. This is the list
   `kickstart distros` prints.
6. **Add tests.** Copy `oraclelinux9_test.go` and adapt the golden-file
   assertions. Render against a fixture `KickstartConfig` and compare with
   `testdata/almalinux9/ks.cfg.golden`.
7. **Update this page** and `profile-guide.md` if the new distro is the
   baseline for a shipped profile.

Golden-file tests are the main gate — the template output is the contract
between proxctl and the installer.

---

## Bootloader requirements

Anaconda + subiquity expect certain bootloader files on the ISO:

| Family | Boot files (BIOS) | Boot files (UEFI) |
|--------|-------------------|-------------------|
| RHEL (OL, Rocky, Alma, RHEL) | `isolinux/isolinux.bin` + `isolinux/boot.cat` | `EFI/BOOT/BOOTX64.EFI` + `images/efiboot.img` |
| Debian (Ubuntu autoinstall)  | `isolinux/isolinux.bin` + `isolinux/boot.cat` | `EFI/BOOT/BOOTX64.EFI` + `boot/grub/grub.cfg` |
| SUSE (SLES AutoYaST)         | `boot/x86_64/loader/isolinux.bin`              | `EFI/BOOT/bootx64.efi` |

proxctl sources these from a **bootloader directory** that defaults to
`/usr/share/xorriso/iso-bootloaders` (Debian/Ubuntu xorriso package) and can
be overridden via `hypervisor.iso.bootloader_dir`. On distros where this
directory does not exist, install `xorriso` from your package manager or
point the override at a local mirror of the files.

---

## Package set differences

`kickstart.packages.base` is installed in the `%packages` stanza of RHEL-family
kickstarts or the equivalent `package_update` / `packages` section of
autoinstall. `kickstart.packages.post` is installed via `dnf install -y` /
`apt install -y` in the `%post` stage.

Minimum sane baselines proxctl recommends per family:

| Family | Recommended base | Recommended post |
|--------|------------------|------------------|
| RHEL   | `chrony`, `cloud-init` (optional), `tar` | `open-vm-tools` (if Proxmox is on vSphere-compat), vendor preinstall pkg |
| Debian | `chrony`, `cloud-init`, `openssh-server` | `qemu-guest-agent` |
| SUSE   | `chrony`, `openssh`, `zypper`            | `qemu-guest-agent` |

Always install `qemu-guest-agent` (or the package name for your distro) so
proxctl can read the VM's IP via the Proxmox `agent` API — without it,
`workflow verify` falls back to ARP scans which are slower and less reliable.
