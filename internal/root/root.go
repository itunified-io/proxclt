// Package root wires all proxctl subcommands into a single Cobra tree.
package root

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
)

// Global flags mirrored across every subcommand.
var (
	flagContext string
	flagStack   string // --stack (preferred; was --env)
	flagEnv     string // --env (deprecated alias; remove in next major)
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
		SilenceUsage:       true,
		PersistentPreRunE:  resolveDeprecatedFlags,
	}

	root.PersistentFlags().StringVar(&flagContext, "context", "", "proxctl context to use (overrides current-context)")
	root.PersistentFlags().StringVar(&flagStack, "stack", "", "stack manifest name or path (overrides current stack)")
	root.PersistentFlags().StringVar(&flagEnv, "env", "", "DEPRECATED: use --stack instead")
	// Hide the deprecated --env flag from `--help` output but keep it accepted.
	_ = root.PersistentFlags().MarkHidden("env")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit JSON on stdout (stderr still carries logs)")
	root.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "assume yes for confirm prompts (DANGEROUS)")

	// One-shot migration of ~/.proxctl/envs.yaml → stacks.yaml at process start.
	// Errors are non-fatal (warned to stderr) — we must not block the CLI for a
	// cosmetic rename.
	migrateStacksRegistry()

	root.AddCommand(
		newConfigCmd(),
		newStackCmd(),
		newEnvCompatCmd(),
		newVMCmd(),
		newSnapshotCmd(),
		newKickstartCmd(),
		newBootCmd(),
		newWorkflowCmd(),
		newLicenseCmd(),
		newReplayCmd(),
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

// envFlagDeprecated ensures the `--env` deprecation warning fires at most once
// per process, even though PersistentPreRunE runs on every invocation chain.
var envFlagDeprecated bool

// resolveDeprecatedFlags maps --env to --stack (with a one-time warning) and
// maps $PROXCTL_ENV to $PROXCTL_STACK (with a one-time warning). It runs as
// the root command's PersistentPreRunE so every subcommand benefits.
func resolveDeprecatedFlags(cmd *cobra.Command, _ []string) error {
	// Detect invocation under the deprecated `env` verb tree by walking the
	// parent chain — cobra's one-PersistentPreRunE-closest-wins rule means the
	// env subtree's warning must live here too.
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "env" && c.Hidden {
			if !envDeprecated {
				fmt.Fprintln(os.Stderr, "DEPRECATED: 'proxctl env' is renamed to 'proxctl stack' and will be removed in the next major release.")
				envDeprecated = true
			}
			break
		}
	}
	if flagEnv != "" {
		if !envFlagDeprecated {
			fmt.Fprintln(os.Stderr, "DEPRECATED: --env is renamed to --stack and will be removed in the next major release.")
			envFlagDeprecated = true
		}
		// --stack takes precedence if both supplied; otherwise promote --env.
		if flagStack == "" {
			flagStack = flagEnv
		}
	}
	// Env-var alias: $PROXCTL_ENV → $PROXCTL_STACK.
	if os.Getenv("PROXCTL_STACK") == "" {
		if v := os.Getenv("PROXCTL_ENV"); v != "" {
			if !envVarDeprecated {
				fmt.Fprintln(os.Stderr, "DEPRECATED: $PROXCTL_ENV is renamed to $PROXCTL_STACK and will be removed in the next major release.")
				envVarDeprecated = true
			}
			_ = os.Setenv("PROXCTL_STACK", v)
		}
	}
	return nil
}

var envVarDeprecated bool

// migrationOnce ensures we only attempt the registry migration once per
// process (New() may be called by tests many times per run).
var migrationOnce sync.Once

// migrateStacksRegistryForTest re-runs the migration logic with a fresh
// sync.Once so tests can exercise the rename repeatedly. Not exported outside
// the package.
func migrateStacksRegistryForTest(t interface{ Helper() }) {
	t.Helper()
	migrationOnce = sync.Once{}
	migrateStacksRegistry()
}

// migrateStacksRegistry performs the one-shot rename of
// ~/.proxctl/envs.yaml → ~/.proxctl/stacks.yaml.
//
// Behaviour:
//   - If stacks.yaml is present, do nothing (even if envs.yaml also exists —
//     in that case we emit a warning about the leftover file).
//   - If only envs.yaml is present, rename it and log a single-line notice.
//   - If neither exists, do nothing.
//
// All errors are reported to stderr but are non-fatal.
func migrateStacksRegistry() {
	migrationOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return
		}
		dir := filepath.Join(home, ".proxctl")
		oldPath := filepath.Join(dir, "envs.yaml")
		newPath := filepath.Join(dir, "stacks.yaml")

		_, newErr := os.Stat(newPath)
		_, oldErr := os.Stat(oldPath)
		newExists := newErr == nil
		oldExists := oldErr == nil

		switch {
		case newExists && oldExists:
			fmt.Fprintf(os.Stderr, "warning: both %s and %s exist; using stacks.yaml — remove envs.yaml to silence this warning\n", newPath, oldPath)
		case !newExists && oldExists:
			if err := os.Rename(oldPath, newPath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not migrate %s → %s: %v\n", oldPath, newPath, err)
				return
			}
			fmt.Fprintf(os.Stderr, "migrated %s → %s\n", oldPath, newPath)
		}
	})
}
