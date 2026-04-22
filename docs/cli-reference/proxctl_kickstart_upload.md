## proxctl kickstart upload

Upload an ISO to a Proxmox storage

```
proxctl kickstart upload FILE [flags]
```

### Options

```
  -h, --help             help for upload
      --node string      PVE node name
      --storage string   PVE storage name
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

