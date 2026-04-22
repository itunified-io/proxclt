## proxctl vm stop

Stop a VM (ACPI shutdown; --force for hard stop)

```
proxctl vm stop NAME [flags]
```

### Options

```
      --force   hard stop instead of ACPI shutdown
  -h, --help    help for stop
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --env string       env manifest name or path (overrides current env)
      --json             emit JSON on stdout (stderr still carries logs)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl vm](proxctl_vm.md)	 - Manage individual VMs (create, start, stop, delete, list, status)

