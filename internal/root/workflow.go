package root

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/itunified-io/proxctl/pkg/kickstart"
	"github.com/itunified-io/proxctl/pkg/workflow"
	"github.com/spf13/cobra"
)

func newWorkflowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "workflow",
		Short: "Multi-VM idempotent orchestration (plan, up, down, status, verify)",
	}

	var wfNode string
	var wfBootloader string
	var wfForce bool
	var wfDryRun bool

	makeWF := func(nodeName string) (*workflow.SingleVMWorkflow, error) {
		env, err := loadEnvManifest("")
		if err != nil {
			return nil, err
		}
		client, err := loadProxmoxClient()
		if err != nil {
			return nil, err
		}
		rnd, err := kickstart.NewRenderer()
		if err != nil {
			return nil, err
		}
		w := &workflow.SingleVMWorkflow{
			Config:   env,
			NodeName: nodeName,
			Client:   client,
			Renderer: rnd,
			DryRun:   wfDryRun,
		}
		if wfBootloader != "" {
			w.Builder = kickstart.NewISOBuilder(wfBootloader)
		}
		return w, nil
	}

	planCmd := &cobra.Command{
		Use:   "plan",
		Short: "Dry-run: print the change set",
		RunE: func(cmd *cobra.Command, args []string) error {
			if wfNode == "" {
				return fmt.Errorf("--node required for Phase 2 workflow")
			}
			w, err := makeWF(wfNode)
			if err != nil {
				return err
			}
			changes, err := w.Plan(context.Background())
			if err != nil {
				return err
			}
			if flagJSON {
				return json.NewEncoder(os.Stdout).Encode(changes)
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
			if wfNode == "" {
				return fmt.Errorf("--node required for Phase 2 workflow")
			}
			w, err := makeWF(wfNode)
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
			if wfNode == "" {
				return fmt.Errorf("--node required for Phase 2 workflow")
			}
			if !flagYes {
				return fmt.Errorf("refusing to down without --yes")
			}
			w, err := makeWF(wfNode)
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
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
			if wfNode == "" {
				return fmt.Errorf("--node required for Phase 2 workflow")
			}
			w, err := makeWF(wfNode)
			if err != nil {
				return err
			}
			return w.Verify(context.Background())
		},
	}

	for _, sub := range []*cobra.Command{planCmd, upCmd, downCmd, verifyCmd} {
		sub.Flags().StringVar(&wfNode, "node", "", "node name from env manifest (required)")
		sub.Flags().StringVar(&wfBootloader, "bootloader-dir", "", "path to bootloader files (isolinux.bin, vmlinuz, initrd.img)")
	}
	upCmd.Flags().BoolVar(&wfDryRun, "dry-run", false, "print actions without executing")
	downCmd.Flags().BoolVar(&wfForce, "force", false, "hard stop instead of ACPI shutdown")

	c.AddCommand(planCmd, upCmd, downCmd, statusCmd, verifyCmd)
	return c
}
