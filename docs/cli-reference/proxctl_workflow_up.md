## proxctl workflow up

Apply the workflow (idempotent)

```
proxctl workflow up [flags]
```

### Options

```
      --bootloader-dir string   path to bootloader files (isolinux.bin, vmlinuz, initrd.img)
      --continue-on-error       keep running remaining nodes when one fails
      --dry-run                 print actions without executing
  -h, --help                    help for up
      --max-concurrency int     cap concurrent per-node Apply goroutines (0=default)
      --node string             node name from env manifest (single-node override)
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --env string       env manifest name or path (overrides current env)
      --json             emit JSON on stdout (stderr still carries logs)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl workflow](proxctl_workflow.md)	 - Multi-VM idempotent orchestration (plan, up, down, status, verify)

