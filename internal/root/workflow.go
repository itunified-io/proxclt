package root

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/itunified-io/proxctl/pkg/config"
	"github.com/itunified-io/proxctl/pkg/kickstart"
	"github.com/itunified-io/proxctl/pkg/workflow"
	"github.com/spf13/cobra"
)

// isMultiNode reports whether the env manifest has more than one node.
func isMultiNode(env *config.Env) bool {
	if env == nil {
		return false
	}
	hyp := env.Spec.Hypervisor.Resolved()
	if hyp == nil {
		return false
	}
	return len(hyp.Nodes) > 1
}

func newWorkflowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "workflow",
		Short: "Multi-VM idempotent orchestration (plan, up, down, status, verify)",
	}

	var wfNode string
	var wfBootloader string
	var wfForce bool
	var wfDryRun bool
	var wfMaxConc int
	var wfContinue bool

	loadCommon := func() (*config.Env, *kickstart.Renderer, *kickstart.ISOBuilder, error) {
		env, err := loadEnvManifest("")
		if err != nil {
			return nil, nil, nil, err
		}
		rnd, err := kickstart.NewRenderer()
		if err != nil {
			return nil, nil, nil, err
		}
		var builder *kickstart.ISOBuilder
		if wfBootloader != "" {
			builder = kickstart.NewISOBuilder(wfBootloader)
		}
		return env, rnd, builder, nil
	}

	makeSingle := func(env *config.Env, rnd *kickstart.Renderer, builder *kickstart.ISOBuilder, nodeName string) (*workflow.SingleVMWorkflow, error) {
		client, err := loadProxmoxClient()
		if err != nil {
			return nil, err
		}
		return &workflow.SingleVMWorkflow{
			Config:   env,
			NodeName: nodeName,
			Client:   client,
			Renderer: rnd,
			Builder:  builder,
			DryRun:   wfDryRun,
		}, nil
	}

	makeMulti := func(env *config.Env, rnd *kickstart.Renderer, builder *kickstart.ISOBuilder) (*workflow.MultiNodeWorkflow, error) {
		client, err := loadProxmoxClient()
		if err != nil {
			return nil, err
		}
		m := workflow.NewMultiNodeWorkflow(env, client, rnd, builder)
		m.DryRun = wfDryRun
		if wfMaxConc > 0 {
			m.MaxConcurrency = wfMaxConc
		}
		m.ContinueOnError = wfContinue
		return m, nil
	}

	planCmd := &cobra.Command{
		Use:   "plan",
		Short: "Dry-run: print the change set",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, rnd, builder, err := loadCommon()
			if err != nil {
				if wfNode == "" {
					return fmt.Errorf("--node required for single-node workflow")
				}
				return err
			}
			if isMultiNode(env) && wfNode == "" {
				m, err := makeMulti(env, rnd, builder)
				if err != nil {
					return err
				}
				plan, err := m.Plan(context.Background())
				if err != nil {
					return err
				}
				if flagJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
				}
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NODE\tKIND\tTARGET\tDESCRIPTION")
				for node, changes := range plan {
					for _, ch := range changes {
						fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", node, ch.Kind, ch.Target, ch.Description)
					}
				}
				return tw.Flush()
			}
			if wfNode == "" {
				return fmt.Errorf("--node required for single-node workflow")
			}
			w, err := makeSingle(env, rnd, builder, wfNode)
			if err != nil {
				return err
			}
			changes, err := w.Plan(context.Background())
			if err != nil {
				return err
			}
			if flagJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(changes)
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KIND\tTARGET\tDESCRIPTION")
			for _, ch := range changes {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", ch.Kind, ch.Target, ch.Description)
			}
			return tw.Flush()
		},
	}

	upCmd := &cobra.Command{
		Use:   "up",
		Short: "Apply the workflow (idempotent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, rnd, builder, err := loadCommon()
			if err != nil {
				if wfNode == "" {
					return fmt.Errorf("--node required for single-node workflow")
				}
				return err
			}
			if isMultiNode(env) && wfNode == "" {
				m, err := makeMulti(env, rnd, builder)
				if err != nil {
					return err
				}
				return m.Up(context.Background())
			}
			if wfNode == "" {
				return fmt.Errorf("--node required for single-node workflow")
			}
			w, err := makeSingle(env, rnd, builder, wfNode)
			if err != nil {
				return err
			}
			return w.Up(context.Background())
		},
	}

	downCmd := &cobra.Command{
		Use:   "down",
		Short: "Tear down the workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, rnd, builder, err := loadCommon()
			if err != nil {
				if wfNode == "" {
					return fmt.Errorf("--node required for single-node workflow")
				}
				if !flagYes {
					return fmt.Errorf("refusing to down without --yes")
				}
				return err
			}
			if !flagYes {
				return fmt.Errorf("refusing to down without --yes")
			}
			if isMultiNode(env) && wfNode == "" {
				m, err := makeMulti(env, rnd, builder)
				if err != nil {
					return err
				}
				return m.Down(context.Background(), wfForce)
			}
			if wfNode == "" {
				return fmt.Errorf("--node required for single-node workflow")
			}
			w, err := makeSingle(env, rnd, builder, wfNode)
			if err != nil {
				return err
			}
			return w.Down(context.Background(), wfForce)
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current VM status for all nodes in env",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadEnvManifest("")
			if err != nil {
				return err
			}
			client, err := loadProxmoxClient()
			if err != nil {
				return err
			}
			hyp := env.Spec.Hypervisor.Resolved()
			if hyp == nil {
				return fmt.Errorf("hypervisor not resolved")
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NODE\tPVE\tVMID\tSTATUS")
			for name, n := range hyp.Nodes {
				vm, err := client.GetVM(context.Background(), n.Proxmox.NodeName, n.Proxmox.VMID)
				status := "absent"
				if err == nil && vm != nil {
					status = vm.Status
				}
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", name, n.Proxmox.NodeName, n.Proxmox.VMID, status)
			}
			return tw.Flush()
		},
	}

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Post-deploy health check",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, rnd, builder, err := loadCommon()
			if err != nil {
				if wfNode == "" {
					return fmt.Errorf("--node required for single-node workflow")
				}
				return err
			}
			if isMultiNode(env) && wfNode == "" {
				m, err := makeMulti(env, rnd, builder)
				if err != nil {
					return err
				}
				return m.Verify(context.Background())
			}
			if wfNode == "" {
				return fmt.Errorf("--node required for single-node workflow")
			}
			w, err := makeSingle(env, rnd, builder, wfNode)
			if err != nil {
				return err
			}
			return w.Verify(context.Background())
		},
	}

	for _, sub := range []*cobra.Command{planCmd, upCmd, downCmd, verifyCmd} {
		sub.Flags().StringVar(&wfNode, "node", "", "node name from env manifest (single-node override)")
		sub.Flags().StringVar(&wfBootloader, "bootloader-dir", "", "path to bootloader files (isolinux.bin, vmlinuz, initrd.img)")
	}
	upCmd.Flags().BoolVar(&wfDryRun, "dry-run", false, "print actions without executing")
	upCmd.Flags().IntVar(&wfMaxConc, "max-concurrency", 0, "cap concurrent per-node Apply goroutines (0=default)")
	upCmd.Flags().BoolVar(&wfContinue, "continue-on-error", false, "keep running remaining nodes when one fails")
	downCmd.Flags().BoolVar(&wfForce, "force", false, "hard stop instead of ACPI shutdown")

	c.AddCommand(planCmd, upCmd, downCmd, statusCmd, verifyCmd, newWorkflowProfileCmd())
	return c
}

// newWorkflowProfileCmd builds the `workflow profile list|show` subtree.
func newWorkflowProfileCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "profile",
		Short: "Inspect the built-in env profile library",
	}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List names of shipped profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			names := config.BuiltinProfiles()
			if flagJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(names)
			}
			for _, n := range names {
				fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "show <name>",
		Short: "Print the raw YAML of a shipped profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := config.LoadBuiltinProfile(args[0])
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	})
	return c
}
