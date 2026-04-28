package root

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/itunified-io/proxctl/pkg/kickstart"
	"github.com/itunified-io/proxctl/pkg/proxmox"
	"github.com/itunified-io/proxctl/pkg/workflow"
	"github.com/spf13/cobra"
)

func newVMCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "vm",
		Short: "Manage individual VMs (create, start, stop, delete, list, status)",
	}

	var vmForceStop bool
	var vmDeletePurge bool

	c.AddCommand(
		func() *cobra.Command {
			var skipKickstart bool
			cc := &cobra.Command{
				Use:   "create NAME",
				Short: "Create a VM from env spec",
				Args:  cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					env, err := loadEnvManifest("")
					if err != nil {
						return err
					}
					client, err := loadProxmoxClient()
					if err != nil {
						return err
					}
					rnd, err := kickstart.NewRenderer()
					if err != nil {
						return err
					}
					w := &workflow.SingleVMWorkflow{
						Config:             env,
						NodeName:           args[0],
						Client:             client,
						Renderer:           rnd,
						SkipKickstartBuild: skipKickstart,
					}
					return w.Up(context.Background())
				},
			}
			cc.Flags().BoolVar(&skipKickstart, "skip-kickstart-build", false,
				"skip render/build/upload of kickstart ISO; assume operator pre-uploaded "+
					"<kickstart_storage>:iso/<NAME>_kickstart.iso (verified at apply time)")
			return cc
		}(),
		&cobra.Command{
			Use:   "start NAME",
			Short: "Start a VM",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				env, err := loadEnvManifest("")
				if err != nil {
					return err
				}
				node, vmid, err := resolveNodeRef(env, args[0])
				if err != nil {
					return err
				}
				client, err := loadProxmoxClient()
				if err != nil {
					return err
				}
				return client.StartVM(context.Background(), node, vmid)
			},
		},
		func() *cobra.Command {
			cc := &cobra.Command{
				Use:   "stop NAME",
				Short: "Stop a VM (ACPI shutdown; --force for hard stop)",
				Args:  cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					env, err := loadEnvManifest("")
					if err != nil {
						return err
					}
					node, vmid, err := resolveNodeRef(env, args[0])
					if err != nil {
						return err
					}
					client, err := loadProxmoxClient()
					if err != nil {
						return err
					}
					return client.StopVM(context.Background(), node, vmid, vmForceStop)
				},
			}
			cc.Flags().BoolVar(&vmForceStop, "force", false, "hard stop instead of ACPI shutdown")
			return cc
		}(),
		&cobra.Command{
			Use:   "reboot NAME",
			Short: "Reboot a VM",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				env, err := loadEnvManifest("")
				if err != nil {
					return err
				}
				node, vmid, err := resolveNodeRef(env, args[0])
				if err != nil {
					return err
				}
				client, err := loadProxmoxClient()
				if err != nil {
					return err
				}
				return client.RebootVM(context.Background(), node, vmid)
			},
		},
		func() *cobra.Command {
			cc := &cobra.Command{
				Use:   "delete NAME",
				Short: "Delete a VM (double-confirm gate)",
				Args:  cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					env, err := loadEnvManifest("")
					if err != nil {
						return err
					}
					node, vmid, err := resolveNodeRef(env, args[0])
					if err != nil {
						return err
					}
					if !flagYes {
						return fmt.Errorf("refusing to delete without --yes")
					}
					client, err := loadProxmoxClient()
					if err != nil {
						return err
					}
					return client.DeleteVM(context.Background(), node, vmid, vmDeletePurge)
				},
			}
			cc.Flags().BoolVar(&vmDeletePurge, "purge", true, "purge disks + references")
			return cc
		}(),
		&cobra.Command{
			Use:   "list",
			Short: "List VMs on the Proxmox node configured in env",
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
				// Collect unique PVE nodes referenced by manifest.
				seen := map[string]bool{}
				var allVMs []proxmox.VM
				for _, n := range hyp.Nodes {
					pveNode := n.Proxmox.NodeName
					if seen[pveNode] {
						continue
					}
					seen[pveNode] = true
					vms, err := client.ListVMs(context.Background(), pveNode)
					if err != nil {
						return err
					}
					allVMs = append(allVMs, vms...)
				}
				if flagJSON {
					return json.NewEncoder(os.Stdout).Encode(allVMs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "VMID\tNAME\tNODE\tSTATUS")
				for _, vm := range allVMs {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", vm.VMID, vm.Name, vm.Node, vm.Status)
				}
				return tw.Flush()
			},
		},
		&cobra.Command{
			Use:   "status NAME",
			Short: "Print live VM status",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				env, err := loadEnvManifest("")
				if err != nil {
					return err
				}
				node, vmid, err := resolveNodeRef(env, args[0])
				if err != nil {
					return err
				}
				client, err := loadProxmoxClient()
				if err != nil {
					return err
				}
				vm, err := client.GetVM(context.Background(), node, vmid)
				if err != nil {
					return err
				}
				if flagJSON {
					return json.NewEncoder(os.Stdout).Encode(vm)
				}
				fmt.Printf("%-10s %d\n%-10s %s\n%-10s %s\n%-10s %s\n%-10s %d MiB\n%-10s %d\n",
					"VMID:", vm.VMID,
					"NAME:", vm.Name,
					"NODE:", vm.Node,
					"STATUS:", vm.Status,
					"MEMORY:", vm.Memory,
					"CORES:", vm.Cores)
				return nil
			},
		},
	)
	return c
}
