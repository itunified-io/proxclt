package root

import "github.com/spf13/cobra"

func newLicenseCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "license",
		Short: "Inspect + activate proxctl license (~/.proxctl/license.jwt)",
	}
	c.AddCommand(
		&cobra.Command{Use: "status", Short: "Current tier, expiry, seats", RunE: notImplemented("license status")},
		&cobra.Command{Use: "activate", Short: "Install a license JWT", RunE: notImplemented("license activate")},
		&cobra.Command{Use: "show", Short: "Pretty-print license claims", RunE: notImplemented("license show")},
		&cobra.Command{Use: "seats-used", Short: "Audit-log-derived seat count", RunE: notImplemented("license seats-used")},
	)
	return c
}
