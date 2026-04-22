package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itunified-io/proxctl/pkg/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print proxctl version, commit, and build date",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "proxctl %s (commit %s, built %s)\n",
				version.Version, version.Commit, version.Date)
			return nil
		},
	}
}
