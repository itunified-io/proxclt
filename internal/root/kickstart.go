package root

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itunified-io/proxctl/pkg/kickstart"
	"github.com/spf13/cobra"
)

func newKickstartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "kickstart",
		Short: "Render kickstart configs, build + upload install ISOs",
	}

	var genNode string
	var genOutDir string
	var buildOut string
	var buildBootloader string
	var uploadStorage string
	var uploadNode string

	c.AddCommand(
		func() *cobra.Command {
			cc := &cobra.Command{
				Use:   "generate [ENV_FILE]",
				Short: "Render kickstart to a file",
				Args:  cobra.MaximumNArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					var envPath string
					if len(args) > 0 {
						envPath = args[0]
					}
					env, err := loadEnvManifest(envPath)
					if err != nil {
						return err
					}
					rnd, err := kickstart.NewRenderer()
					if err != nil {
						return err
					}
					nodes := []string{genNode}
					if genNode == "" {
						hyp := env.Spec.Hypervisor.Resolved()
						if hyp == nil {
							return fmt.Errorf("hypervisor not resolved")
						}
						nodes = nil
						for n := range hyp.Nodes {
							nodes = append(nodes, n)
						}
					}
					if genOutDir == "" {
						genOutDir = "."
					}
					if err := os.MkdirAll(genOutDir, 0o755); err != nil {
						return err
					}
					for _, n := range nodes {
						content, err := rnd.Render(env, n)
						if err != nil {
							return fmt.Errorf("render %s: %w", n, err)
						}
						out := filepath.Join(genOutDir, n+".ks")
						if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
							return err
						}
						fmt.Fprintf(os.Stderr, "wrote %s\n", out)
					}
					return nil
				},
			}
			cc.Flags().StringVar(&genNode, "node", "", "single node name (default: render all)")
			cc.Flags().StringVarP(&genOutDir, "out", "o", ".", "output directory")
			return cc
		}(),
		func() *cobra.Command {
			cc := &cobra.Command{
				Use:   "build-iso KSFILE",
				Short: "Remaster install ISO with the rendered kickstart",
				Args:  cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					ksData, err := os.ReadFile(args[0])
					if err != nil {
						return err
					}
					if buildBootloader == "" {
						return fmt.Errorf("--bootloader-dir required")
					}
					b := kickstart.NewISOBuilder(buildBootloader)
					hostname := strings.TrimSuffix(filepath.Base(args[0]), ".ks")
					path, err := b.Build(string(ksData), hostname)
					if err != nil {
						return err
					}
					if buildOut != "" && buildOut != path {
						if err := os.Rename(path, buildOut); err != nil {
							return err
						}
						path = buildOut
					}
					fmt.Println(path)
					return nil
				},
			}
			cc.Flags().StringVar(&buildBootloader, "bootloader-dir", "", "directory containing isolinux.bin + vmlinuz + initrd.img")
			cc.Flags().StringVarP(&buildOut, "out", "o", "", "output ISO path (default: tempdir)")
			return cc
		}(),
		func() *cobra.Command {
			cc := &cobra.Command{
				Use:   "upload FILE",
				Short: "Upload an ISO to a Proxmox storage",
				Args:  cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					if uploadStorage == "" || uploadNode == "" {
						return fmt.Errorf("--storage and --node required")
					}
					client, err := loadProxmoxClient()
					if err != nil {
						return err
					}
					return client.UploadISO(context.Background(), uploadNode, uploadStorage, args[0], filepath.Base(args[0]))
				},
			}
			cc.Flags().StringVar(&uploadStorage, "storage", "", "PVE storage name")
			cc.Flags().StringVar(&uploadNode, "node", "", "PVE node name")
			return cc
		}(),
		&cobra.Command{
			Use:   "distros",
			Short: "List supported distros",
			RunE: func(cmd *cobra.Command, args []string) error {
				rnd, err := kickstart.NewRenderer()
				if err != nil {
					return err
				}
				for _, d := range rnd.SupportedDistros() {
					fmt.Println(d)
				}
				return nil
			},
		},
	)
	return c
}
