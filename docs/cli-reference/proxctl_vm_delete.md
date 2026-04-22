## proxctl vm delete

Delete a VM (double-confirm gate)

```
proxctl vm delete NAME [flags]
```

### Options

```
  -h, --help    help for delete
      --purge   purge disks + references (default true)
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

