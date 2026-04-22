## proxctl

Proxmox VM provisioning CLI — kickstart, lifecycle, workflows

### Synopsis

proxctl is a standalone Go binary for Proxmox VM provisioning.

See docs/ for the full user guide, configuration reference, and licensing model.

### Options

```
      --context string   proxctl context to use (overrides current-context)
  -h, --help             help for proxctl
      --json             emit JSON on stdout (stderr still carries logs)
      --stack string     stack manifest name or path (overrides current stack)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl boot](proxctl_boot.md)	 - Configure first-boot ISO + post-install ISO ejection
* [proxctl config](proxctl_config.md)	 - Manage proxctl contexts and env manifests
* [proxctl kickstart](proxctl_kickstart.md)	 - Render kickstart configs, build + upload install ISOs
* [proxctl license](proxctl_license.md)	 - Inspect + activate proxctl license (~/.proxctl/license.jwt)
* [proxctl snapshot](proxctl_snapshot.md)	 - Manage VM snapshots
* [proxctl stack](proxctl_stack.md)	 - Manage stack bookmarks (~/.proxctl/stacks.yaml)
* [proxctl version](proxctl_version.md)	 - Print proxctl version, commit, and build date
* [proxctl vm](proxctl_vm.md)	 - Manage individual VMs (create, start, stop, delete, list, status)
* [proxctl workflow](proxctl_workflow.md)	 - Multi-VM idempotent orchestration (plan, up, down, status, verify)

