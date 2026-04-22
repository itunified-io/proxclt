## proxctl vm

Manage individual VMs (create, start, stop, delete, list, status)

### Options

```
  -h, --help   help for vm
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --env string       env manifest name or path (overrides current env)
      --json             emit JSON on stdout (stderr still carries logs)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl](proxctl.md)	 - Proxmox VM provisioning CLI — kickstart, lifecycle, workflows
* [proxctl vm create](proxctl_vm_create.md)	 - Create a VM from env spec
* [proxctl vm delete](proxctl_vm_delete.md)	 - Delete a VM (double-confirm gate)
* [proxctl vm list](proxctl_vm_list.md)	 - List VMs on the Proxmox node configured in env
* [proxctl vm reboot](proxctl_vm_reboot.md)	 - Reboot a VM
* [proxctl vm start](proxctl_vm_start.md)	 - Start a VM
* [proxctl vm status](proxctl_vm_status.md)	 - Print live VM status
* [proxctl vm stop](proxctl_vm_stop.md)	 - Stop a VM (ACPI shutdown; --force for hard stop)

