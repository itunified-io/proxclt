## proxctl kickstart build-iso

Remaster install ISO with the rendered kickstart

```
proxctl kickstart build-iso KSFILE [flags]
```

### Options

```
      --bootloader-dir string   directory containing isolinux.bin + vmlinuz + initrd.img
  -h, --help                    help for build-iso
  -o, --out string              output ISO path (default: tempdir)
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --json             emit JSON on stdout (stderr still carries logs)
      --stack string     stack manifest name or path (overrides current stack)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl kickstart](proxctl_kickstart.md)	 - Render kickstart configs, build + upload install ISOs

