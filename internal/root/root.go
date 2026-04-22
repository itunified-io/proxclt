// Package root wires all proxctl subcommands into a single Cobra tree.
package root

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Global flags mirrored across every subcommand.
var (
	flagContext string
	flagEnv     string
	flagJSON    bool
	flagYes     bool
)

// New returns a fully configured root command.
func New() *cobra.Command {
	root := &cobra.Command{
		Use:   "proxctl",
		Short: "Proxmox VM provisioning CLI — kickstart, lifecycle, workflows",
		Long: `proxctl is a standalone Go binary for Proxmox VM provisioning.

See docs/ for the full user guide, configuration reference, and licensing model.`,
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVar(&flagContext, "context", "", "proxctl context to use (overrides current-context)")
	root.PersistentFlags().StringVar(&flagEnv, "env", "", "env manifest name or path (overrides current env)")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit JSON on stdout (stderr still carries logs)")
	root.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "assume yes for confirm prompts (DANGEROUS)")

	root.AddCommand(
		newConfigCmd(),
		newEnvCmd(),
		newVMCmd(),
		newSnapshotCmd(),
		newKickstartCmd(),
		newBootCmd(),
		newWorkflowCmd(),
		newLicenseCmd(),
		newVersionCmd(),
	)
	return root
}

// osExit is indirected for tests.
var osExit = os.Exit

// Execute runs the root command; intended to be called from main.
func Execute() {
	if err := New().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		osExit(1)
	}
}

// notImplemented is the standard RunE body for every scaffold subcommand.
func notImplemented(use string) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("%s: not implemented yet (scaffold — Phase 2+)", use)
	}
}
