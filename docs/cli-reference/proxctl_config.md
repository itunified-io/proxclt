## proxctl config

Manage proxctl contexts and env manifests

### Options

```
  -h, --help   help for config
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
* [proxctl config current-context](proxctl_config_current-context.md)	 - Print current context
* [proxctl config get-contexts](proxctl_config_get-contexts.md)	 - List all configured contexts
* [proxctl config render](proxctl_config_render.md)	 - Render composed env YAML with $refs resolved and deferred secrets redacted
* [proxctl config schema](proxctl_config_schema.md)	 - Print the JSON Schema for the env manifest
* [proxctl config use-context](proxctl_config_use-context.md)	 - Switch current context
* [proxctl config validate](proxctl_config_validate.md)	 - Validate an env manifest (loads, resolves $refs, checks schema + cross-field rules)

