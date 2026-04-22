package root

import (
	"github.com/spf13/cobra"
)

// newStackCmd builds the `proxctl stack` subcommand tree. The name "stack"
// replaces the older "env" verb (see #15); "env" is kept as a hidden alias
// that emits a deprecation warning on first use.
func newStackCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stack",
		Short: "Manage stack bookmarks (~/.proxctl/stacks.yaml)",
		// NOTE: we deliberately do NOT set `Aliases: []string{"env"}` — the
		// deprecated `env` verb lives as a sibling hidden cobra.Command
		// (newEnvCompatCmd) so we can emit a stderr warning before dispatch.
	}
	c.AddCommand(
		&cobra.Command{Use: "new NAME", Short: "Scaffold a new stack directory", Args: cobra.ExactArgs(1), RunE: notImplemented("stack new")},
		&cobra.Command{Use: "list", Short: "List bookmarked stacks", RunE: notImplemented("stack list")},
		&cobra.Command{Use: "use NAME", Short: "Switch current stack bookmark", Args: cobra.ExactArgs(1), RunE: notImplemented("stack use")},
		&cobra.Command{Use: "current", Short: "Print current stack", RunE: notImplemented("stack current")},
		&cobra.Command{Use: "add NAME", Short: "Bookmark a stack (local path or git ref)", Args: cobra.ExactArgs(1), RunE: notImplemented("stack add")},
		&cobra.Command{Use: "remove NAME", Short: "Remove a stack bookmark", Args: cobra.ExactArgs(1), RunE: notImplemented("stack remove")},
		&cobra.Command{Use: "show [NAME]", Short: "Show resolved paths + sha of stack", Args: cobra.MaximumNArgs(1), RunE: notImplemented("stack show")},
	)
	return c
}

// newEnvCompatCmd builds a compatibility "env" subcommand tree that proxies
// to the "stack" tree and prints a deprecation warning on first use.
//
// This is a sibling of newStackCmd (NOT an Aliases entry on stack) because
// the alias form cannot emit a warning or be hidden separately. It will be
// removed in the next major release.
func newEnvCompatCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "env",
		Short:  "DEPRECATED: use `proxctl stack` instead",
		Hidden: true,
		// The deprecation warning is emitted by the root command's
		// PersistentPreRunE (resolveDeprecatedFlags) because cobra invokes only
		// the closest PersistentPreRunE in the parent chain — a hook installed
		// here would be shadowed by root's.
	}
	c.AddCommand(
		&cobra.Command{Use: "new NAME", Short: "Scaffold a new stack directory", Args: cobra.ExactArgs(1), RunE: notImplemented("stack new")},
		&cobra.Command{Use: "list", Short: "List bookmarked stacks", RunE: notImplemented("stack list")},
		&cobra.Command{Use: "use NAME", Short: "Switch current stack bookmark", Args: cobra.ExactArgs(1), RunE: notImplemented("stack use")},
		&cobra.Command{Use: "current", Short: "Print current stack", RunE: notImplemented("stack current")},
		&cobra.Command{Use: "add NAME", Short: "Bookmark a stack (local path or git ref)", Args: cobra.ExactArgs(1), RunE: notImplemented("stack add")},
		&cobra.Command{Use: "remove NAME", Short: "Remove a stack bookmark", Args: cobra.ExactArgs(1), RunE: notImplemented("stack remove")},
		&cobra.Command{Use: "show [NAME]", Short: "Show resolved paths + sha of stack", Args: cobra.MaximumNArgs(1), RunE: notImplemented("stack show")},
	)
	return c
}

// envDeprecated tracks whether we've already emitted the `env`-verb warning
// this process (exactly one stderr line per invocation).
var envDeprecated bool
