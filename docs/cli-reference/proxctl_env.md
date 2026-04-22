## proxctl env

Manage env bookmarks (~/.proxctl/envs.yaml)

### Options

```
  -h, --help   help for env
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
* [proxctl env add](proxctl_env_add.md)	 - Bookmark an env (local path or git ref)
* [proxctl env current](proxctl_env_current.md)	 - Print current env
* [proxctl env list](proxctl_env_list.md)	 - List bookmarked envs
* [proxctl env new](proxctl_env_new.md)	 - Scaffold a new env directory
* [proxctl env remove](proxctl_env_remove.md)	 - Remove an env bookmark
* [proxctl env show](proxctl_env_show.md)	 - Show resolved paths + sha of env
* [proxctl env use](proxctl_env_use.md)	 - Switch current env bookmark

