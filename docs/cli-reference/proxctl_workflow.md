## proxctl workflow

Multi-VM idempotent orchestration (plan, up, down, status, verify)

### Options

```
  -h, --help   help for workflow
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
* [proxctl workflow down](proxctl_workflow_down.md)	 - Tear down the workflow
* [proxctl workflow plan](proxctl_workflow_plan.md)	 - Dry-run: print the change set
* [proxctl workflow profile](proxctl_workflow_profile.md)	 - Inspect the built-in env profile library
* [proxctl workflow status](proxctl_workflow_status.md)	 - Show current VM status for all nodes in env
* [proxctl workflow up](proxctl_workflow_up.md)	 - Apply the workflow (idempotent)
* [proxctl workflow verify](proxctl_workflow_verify.md)	 - Post-deploy health check

