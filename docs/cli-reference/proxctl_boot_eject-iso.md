## proxctl boot eject-iso

Detach the install ISO after first boot

```
proxctl boot eject-iso NAME [flags]
```

### Options

```
  -h, --help         help for eject-iso
      --ide string   ide slot to eject (default "ide3")
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

