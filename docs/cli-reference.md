# CLI reference

Auto-generated from the Cobra command tree via `make docs-cli`. Run it to
refresh `docs/cli-reference/` after changing any command or flag:

```bash
make docs-cli
git add docs/cli-reference && git commit -m "docs: regenerate CLI reference"
```

The generator lives at [`cmd/docgen/main.go`](../cmd/docgen/main.go). Each
leaf command gets its own Markdown page under
[`docs/cli-reference/`](cli-reference/), cross-linked with parent + sibling
commands. The root command lives at
[`cli-reference/proxctl.md`](cli-reference/proxctl.md).

## Command tree

### Top-level

- [`proxctl`](cli-reference/proxctl.md)
- [`proxctl version`](cli-reference/proxctl_version.md)

### `proxctl config`

- [`proxctl config`](cli-reference/proxctl_config.md)
- [`proxctl config validate`](cli-reference/proxctl_config_validate.md)
- [`proxctl config render`](cli-reference/proxctl_config_render.md)
- [`proxctl config schema`](cli-reference/proxctl_config_schema.md)
- [`proxctl config use-context`](cli-reference/proxctl_config_use-context.md)
- [`proxctl config current-context`](cli-reference/proxctl_config_current-context.md)
- [`proxctl config get-contexts`](cli-reference/proxctl_config_get-contexts.md)

### `proxctl stack`

Renamed from `proxctl env` in v2026.04.11.8 (#15). The `env` verb remains as a
deprecated alias for one release.

- [`proxctl stack`](cli-reference/proxctl_stack.md)
- [`proxctl stack new`](cli-reference/proxctl_stack_new.md)
- [`proxctl stack list`](cli-reference/proxctl_stack_list.md)
- [`proxctl stack use`](cli-reference/proxctl_stack_use.md)
- [`proxctl stack current`](cli-reference/proxctl_stack_current.md)
- [`proxctl stack add`](cli-reference/proxctl_stack_add.md)
- [`proxctl stack remove`](cli-reference/proxctl_stack_remove.md)
- [`proxctl stack show`](cli-reference/proxctl_stack_show.md)

### `proxctl vm`

- [`proxctl vm`](cli-reference/proxctl_vm.md)
- [`proxctl vm create`](cli-reference/proxctl_vm_create.md)
- [`proxctl vm start`](cli-reference/proxctl_vm_start.md)
- [`proxctl vm stop`](cli-reference/proxctl_vm_stop.md)
- [`proxctl vm reboot`](cli-reference/proxctl_vm_reboot.md)
- [`proxctl vm delete`](cli-reference/proxctl_vm_delete.md)
- [`proxctl vm list`](cli-reference/proxctl_vm_list.md)
- [`proxctl vm status`](cli-reference/proxctl_vm_status.md)

### `proxctl snapshot`

- [`proxctl snapshot`](cli-reference/proxctl_snapshot.md)
- [`proxctl snapshot create`](cli-reference/proxctl_snapshot_create.md)
- [`proxctl snapshot restore`](cli-reference/proxctl_snapshot_restore.md)
- [`proxctl snapshot list`](cli-reference/proxctl_snapshot_list.md)
- [`proxctl snapshot delete`](cli-reference/proxctl_snapshot_delete.md)

### `proxctl kickstart`

- [`proxctl kickstart`](cli-reference/proxctl_kickstart.md)
- [`proxctl kickstart generate`](cli-reference/proxctl_kickstart_generate.md)
- [`proxctl kickstart build-iso`](cli-reference/proxctl_kickstart_build-iso.md)
- [`proxctl kickstart upload`](cli-reference/proxctl_kickstart_upload.md)
- [`proxctl kickstart distros`](cli-reference/proxctl_kickstart_distros.md)

### `proxctl boot`

- [`proxctl boot`](cli-reference/proxctl_boot.md)
- [`proxctl boot configure-first-boot`](cli-reference/proxctl_boot_configure-first-boot.md)
- [`proxctl boot eject-iso`](cli-reference/proxctl_boot_eject-iso.md)

### `proxctl workflow`

- [`proxctl workflow`](cli-reference/proxctl_workflow.md)
- [`proxctl workflow plan`](cli-reference/proxctl_workflow_plan.md)
- [`proxctl workflow up`](cli-reference/proxctl_workflow_up.md)
- [`proxctl workflow down`](cli-reference/proxctl_workflow_down.md)
- [`proxctl workflow status`](cli-reference/proxctl_workflow_status.md)
- [`proxctl workflow verify`](cli-reference/proxctl_workflow_verify.md)
- [`proxctl workflow profile`](cli-reference/proxctl_workflow_profile.md)
- [`proxctl workflow profile list`](cli-reference/proxctl_workflow_profile_list.md)
- [`proxctl workflow profile show`](cli-reference/proxctl_workflow_profile_show.md)

### `proxctl license`

- [`proxctl license`](cli-reference/proxctl_license.md)
- [`proxctl license status`](cli-reference/proxctl_license_status.md)
- [`proxctl license activate`](cli-reference/proxctl_license_activate.md)
- [`proxctl license show`](cli-reference/proxctl_license_show.md)
- [`proxctl license seats-used`](cli-reference/proxctl_license_seats-used.md)
