package root

import (
	"fmt"
	"strings"

	"github.com/itunified-io/proxclt/pkg/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Manage proxclt contexts and env manifests",
	}

	validateCmd := &cobra.Command{
		Use:   "validate PATH",
		Short: "Validate an env manifest (loads, resolves $refs, checks schema + cross-field rules)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := config.Load(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK: %s (version=%s, kind=%s)\n", env.Metadata.Name, env.Version, env.Kind)
			return nil
		},
	}

	renderCmd := &cobra.Command{
		Use:   "render PATH",
		Short: "Render composed env YAML with $refs resolved and deferred secrets redacted",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := config.Load(args[0])
			if err != nil {
				return err
			}
			redactDeferredSecrets(env)
			b, err := yaml.Marshal(env)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}

	schemaCmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema for the env manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := config.GenerateSchema()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), s)
			return nil
		},
	}

	c.AddCommand(
		validateCmd,
		renderCmd,
		schemaCmd,
		&cobra.Command{Use: "use-context NAME", Short: "Switch current context", Args: cobra.ExactArgs(1), RunE: notImplemented("config use-context")},
		&cobra.Command{Use: "current-context", Short: "Print current context", RunE: notImplemented("config current-context")},
		&cobra.Command{Use: "get-contexts", Short: "List all configured contexts", RunE: notImplemented("config get-contexts")},
	)
	return c
}

// redactDeferredSecrets replaces <VAULT:…> / <GEN_SSH_KEY:…> markers with
// <REDACTED> in every string field of env.
func redactDeferredSecrets(env *config.Env) {
	config.WalkStringsForTest(env, func(s string) string {
		out := s
		for _, prefix := range []string{"<VAULT:", "<GEN_SSH_KEY:"} {
			for {
				i := strings.Index(out, prefix)
				if i < 0 {
					break
				}
				j := strings.Index(out[i:], ">")
				if j < 0 {
					break
				}
				out = out[:i] + "<REDACTED>" + out[i+j+1:]
			}
		}
		return out
	})
}
