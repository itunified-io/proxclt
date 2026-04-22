package root

import "github.com/spf13/cobra"

func newEnvCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "env",
		Short: "Manage env bookmarks (~/.proxctl/envs.yaml)",
	}
	c.AddCommand(
		&cobra.Command{Use: "new NAME", Short: "Scaffold a new env directory", Args: cobra.ExactArgs(1), RunE: notImplemented("env new")},
		&cobra.Command{Use: "list", Short: "List bookmarked envs", RunE: notImplemented("env list")},
		&cobra.Command{Use: "use NAME", Short: "Switch current env bookmark", Args: cobra.ExactArgs(1), RunE: notImplemented("env use")},
		&cobra.Command{Use: "current", Short: "Print current env", RunE: notImplemented("env current")},
		&cobra.Command{Use: "add NAME", Short: "Bookmark an env (local path or git ref)", Args: cobra.ExactArgs(1), RunE: notImplemented("env add")},
		&cobra.Command{Use: "remove NAME", Short: "Remove an env bookmark", Args: cobra.ExactArgs(1), RunE: notImplemented("env remove")},
		&cobra.Command{Use: "show [NAME]", Short: "Show resolved paths + sha of env", Args: cobra.MaximumNArgs(1), RunE: notImplemented("env show")},
	)
	return c
}
