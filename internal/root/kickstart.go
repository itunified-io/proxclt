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
	var ubuntuNode string
	var ubuntuSourceISO string
	var ubuntuOut string
	var ubuntuWorkDir string

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
		func() *cobra.Command {
			cc := &cobra.Command{
				Use:   "build-ubuntu [ENV_FILE]",
				Short: "Build Ubuntu 22.04+ Subiquity autoinstall ISO (cidata + GRUB cmdline injection)",
				Long: `Renders user-data + meta-data from the env manifest, embeds them at /cidata/
in a remastered copy of the upstream Ubuntu live-server install ISO, and
patches the GRUB linux/linuxefi/isolinux append lines to add
` + "`autoinstall ds=nocloud\\;s=/cidata/`" + ` to the kernel cmdline. The
result is a standalone ISO that boots Ubuntu unattended via Subiquity.

Use this when env.spec.hypervisor.kickstart.distro = "ubuntu2404".`,
				Args: cobra.MaximumNArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					if ubuntuSourceISO == "" {
						return fmt.Errorf("--source-iso required (path to upstream Ubuntu live-server ISO)")
					}
					if ubuntuNode == "" {
						return fmt.Errorf("--node required")
					}
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
					userData, err := rnd.RenderTemplate(env, ubuntuNode, "user-data.tmpl")
					if err != nil {
						return fmt.Errorf("render user-data: %w", err)
					}
					metaData, err := rnd.RenderTemplate(env, ubuntuNode, "meta-data.tmpl")
					if err != nil {
						return fmt.Errorf("render meta-data: %w", err)
					}
					cidataDir, err := os.MkdirTemp("", "proxctl-ubuntu-cidata-")
					if err != nil {
						return err
					}
					defer os.RemoveAll(cidataDir)
					if err := os.WriteFile(filepath.Join(cidataDir, "user-data"), []byte(userData), 0o644); err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(cidataDir, "meta-data"), []byte(metaData), 0o644); err != nil {
						return err
					}
					b := kickstart.NewUbuntuISOBuilder(ubuntuSourceISO, cidataDir)
					if ubuntuWorkDir != "" {
						b.WorkDir = ubuntuWorkDir
					}
					path, err := b.Build(ubuntuNode)
					if err != nil {
						return err
					}
					if ubuntuOut != "" && ubuntuOut != path {
						if err := os.Rename(path, ubuntuOut); err != nil {
							return err
						}
						path = ubuntuOut
					}
					fmt.Println(path)
					return nil
				},
			}
			cc.Flags().StringVar(&ubuntuNode, "node", "", "Node name from hypervisor.nodes (used as hostname)")
			cc.Flags().StringVar(&ubuntuSourceISO, "source-iso", "", "Path to upstream Ubuntu live-server install ISO")
			cc.Flags().StringVarP(&ubuntuOut, "out", "o", "", "Output ISO path (default: tempdir under workdir)")
			cc.Flags().StringVar(&ubuntuWorkDir, "workdir", "", "Scratch directory (default: $TMPDIR)")
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
