## proxctl workflow down

Tear down the workflow

```
proxctl workflow down [flags]
```

### Options

```
      --bootloader-dir string   path to bootloader files (isolinux.bin, vmlinuz, initrd.img)
      --force                   hard stop instead of ACPI shutdown
  -h, --help                    help for down
      --node string             node name from env manifest (single-node override)
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --json             emit JSON on stdout (stderr still carries logs)
      --stack string     stack manifest name or path (overrides current stack)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl workflow](proxctl_workflow.md)	 - Multi-VM idempotent orchestration (plan, up, down, status, verify)

