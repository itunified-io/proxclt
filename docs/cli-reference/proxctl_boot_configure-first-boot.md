## proxctl boot configure-first-boot

Attach the kickstart ISO and set boot order

```
proxctl boot configure-first-boot NAME [flags]
```

### Options

```
  -h, --help           help for configure-first-boot
      --ide string     ide slot to attach the ISO to (default "ide3")
      --iso string     PVE ISO ref (e.g. local:iso/kickstart.iso)
      --order string   boot order string (default "ide3;scsi0")
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --env string       env manifest name or path (overrides current env)
      --json             emit JSON on stdout (stderr still carries logs)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl boot](proxctl_boot.md)	 - Configure first-boot ISO + post-install ISO ejection

