## proxctl kickstart

Render kickstart configs, build + upload install ISOs

### Options

```
  -h, --help   help for kickstart
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --json             emit JSON on stdout (stderr still carries logs)
      --stack string     stack manifest name or path (overrides current stack)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl](proxctl.md)	 - Proxmox VM provisioning CLI — kickstart, lifecycle, workflows
* [proxctl kickstart build-iso](proxctl_kickstart_build-iso.md)	 - Remaster install ISO with the rendered kickstart
* [proxctl kickstart distros](proxctl_kickstart_distros.md)	 - List supported distros
* [proxctl kickstart generate](proxctl_kickstart_generate.md)	 - Render kickstart to a file
* [proxctl kickstart upload](proxctl_kickstart_upload.md)	 - Upload an ISO to a Proxmox storage

