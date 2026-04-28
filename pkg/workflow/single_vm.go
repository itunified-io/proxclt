// Package workflow orchestrates multi-step VM provisioning (plan/apply/verify/rollback).
package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/itunified-io/proxctl/pkg/config"
	"github.com/itunified-io/proxctl/pkg/kickstart"
	"github.com/itunified-io/proxctl/pkg/proxmox"
)

// Change is a single unit of work in the plan.
type Change struct {
	Kind        string
	Target      string
	Description string
	Details     map[string]any
}

// SingleVMWorkflow provisions exactly one node from an env manifest.
type SingleVMWorkflow struct {
	Config   *config.Env
	NodeName string
	Client   *proxmox.Client
	Renderer *kickstart.Renderer
	Builder  *kickstart.ISOBuilder
	DryRun   bool
	// UploadMu serializes the upload-iso step across callers that share a
	// Proxmox storage endpoint (multi-node Apply). Optional.
	UploadMu *sync.Mutex
	// SkipKickstartBuild — when true, the workflow skips the
	// render-kickstart, build-iso, and upload-iso steps and goes straight
	// to create-vm + start-vm. The kickstart ISO MUST already be present at
	// `iso.kickstart_storage:<expected-volid>` (verified at plan time).
	// Use this when the operator built and uploaded the kickstart ISO
	// out-of-band (e.g. an OEMDRV-labeled ISO), or when iterating on VM
	// hardware config without re-rendering kickstart. See #23.
	SkipKickstartBuild bool
}

// resolved pulls commonly-accessed nested structures from the env.
type resolved struct {
	hyp     *config.Hypervisor
	node    config.Node
	ks      *config.KickstartConfig
	iso     *config.ISOConfig
	cluster *config.Cluster
}

func (w *SingleVMWorkflow) resolve() (*resolved, error) {
	if w.Config == nil {
		return nil, errors.New("workflow: Config is nil")
	}
	hyp := w.Config.Spec.Hypervisor.Resolved()
	if hyp == nil {
		return nil, errors.New("workflow: hypervisor not resolved")
	}
	node, ok := hyp.Nodes[w.NodeName]
	if !ok {
		return nil, fmt.Errorf("workflow: node %q not in manifest", w.NodeName)
	}
	r := &resolved{
		hyp:  hyp,
		node: node,
		ks:   hyp.Kickstart,
		iso:  hyp.ISO,
	}
	if w.Config.Spec.Cluster != nil {
		r.cluster = w.Config.Spec.Cluster.Resolved()
	}
	return r, nil
}

// Plan computes the change set without side effects.
func (w *SingleVMWorkflow) Plan(ctx context.Context) ([]Change, error) {
	r, err := w.resolve()
	if err != nil {
		return nil, err
	}

	changes := []Change{}

	// 1. Check VM doesn't exist.
	if w.Client != nil {
		exists, err := w.Client.VMExists(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID)
		if err != nil {
			return nil, fmt.Errorf("plan: vm-exists check: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("plan: vm %d already exists on %s (run vm.delete first)",
				r.node.Proxmox.VMID, r.node.Proxmox.NodeName)
		}
	}

	if !w.SkipKickstartBuild {
		changes = append(changes,
			Change{Kind: "render-kickstart", Target: w.NodeName,
				Description: fmt.Sprintf("render %s kickstart for %s", safeDistro(r.ks), w.NodeName)},
			Change{Kind: "build-iso", Target: w.NodeName,
				Description: fmt.Sprintf("build kickstart ISO for %s", w.NodeName)},
		)

		if r.iso != nil && r.iso.KickstartStorage != "" {
			changes = append(changes, Change{
				Kind:        "upload-iso",
				Target:      r.iso.KickstartStorage,
				Description: fmt.Sprintf("upload kickstart ISO to storage %s on node %s", r.iso.KickstartStorage, r.node.Proxmox.NodeName),
			})
		}
	} else {
		// Pre-condition: kickstart ISO must already be present on Proxmox storage.
		// Surface in the plan so operators see what's being assumed; verified
		// at apply time via Client.StorageContentExists if Client is available.
		if r.iso != nil && r.iso.KickstartStorage != "" {
			changes = append(changes, Change{
				Kind:        "verify-kickstart-iso",
				Target:      r.iso.KickstartStorage,
				Description: fmt.Sprintf("verify kickstart ISO already uploaded to %s on node %s (skip-kickstart-build)", r.iso.KickstartStorage, r.node.Proxmox.NodeName),
			})
		}
	}

	changes = append(changes,
		Change{Kind: "create-vm", Target: fmt.Sprintf("%s/%d", r.node.Proxmox.NodeName, r.node.Proxmox.VMID),
			Description: fmt.Sprintf("create VM %d on %s", r.node.Proxmox.VMID, r.node.Proxmox.NodeName),
			Details: map[string]any{
				"memory": safeMemory(r.node.Resources),
				"cores":  safeCores(r.node.Resources),
				"disks":  len(r.node.Disks),
				"nics":   len(r.node.NICs),
			},
		},
		Change{Kind: "start-vm", Target: fmt.Sprintf("%s/%d", r.node.Proxmox.NodeName, r.node.Proxmox.VMID),
			Description: fmt.Sprintf("start VM %d", r.node.Proxmox.VMID)},
	)
	return changes, nil
}

// Apply executes the given change set.
func (w *SingleVMWorkflow) Apply(ctx context.Context, changes []Change) error {
	r, err := w.resolve()
	if err != nil {
		return err
	}

	var renderedKS string
	var isoPath string

	for _, ch := range changes {
		if w.DryRun {
			fmt.Fprintf(os.Stderr, "[dry-run] %s: %s\n", ch.Kind, ch.Description)
			continue
		}
		switch ch.Kind {
		case "render-kickstart":
			if w.Renderer == nil {
				return errors.New("apply: Renderer not set")
			}
			renderedKS, err = w.Renderer.Render(w.Config, w.NodeName)
			if err != nil {
				return fmt.Errorf("render: %w", err)
			}
		case "build-iso":
			if w.Builder == nil {
				return errors.New("apply: Builder not set")
			}
			isoPath, err = w.Builder.Build(renderedKS, w.NodeName)
			if err != nil {
				return fmt.Errorf("build-iso: %w", err)
			}
		case "upload-iso":
			if w.Client == nil {
				return errors.New("apply: Client not set")
			}
			if r.iso == nil || r.iso.KickstartStorage == "" {
				return errors.New("apply: iso.kickstart_storage not configured")
			}
			if isoPath == "" {
				return errors.New("apply: iso path empty (did build-iso run?)")
			}
			if w.UploadMu != nil {
				w.UploadMu.Lock()
			}
			err := w.Client.UploadISO(ctx, r.node.Proxmox.NodeName, r.iso.KickstartStorage, isoPath, fmt.Sprintf("proxctl-%s.iso", w.NodeName))
			if w.UploadMu != nil {
				w.UploadMu.Unlock()
			}
			if err != nil {
				return fmt.Errorf("upload-iso: %w", err)
			}
		case "verify-kickstart-iso":
			if w.Client == nil {
				return errors.New("apply: Client not set")
			}
			if r.iso == nil || r.iso.KickstartStorage == "" {
				return errors.New("apply: iso.kickstart_storage not configured (cannot verify pre-uploaded kickstart ISO)")
			}
			expectedVolID := fmt.Sprintf("%s:iso/%s_kickstart.iso", r.iso.KickstartStorage, w.NodeName)
			present, err := w.Client.StorageContentExists(ctx, r.node.Proxmox.NodeName, r.iso.KickstartStorage, expectedVolID)
			if err != nil {
				return fmt.Errorf("verify-kickstart-iso: %w", err)
			}
			if !present {
				return fmt.Errorf("verify-kickstart-iso: %s not present on %s — upload it first or omit --skip-kickstart-build", expectedVolID, r.node.Proxmox.NodeName)
			}
		case "create-vm":
			if w.Client == nil {
				return errors.New("apply: Client not set")
			}
			opts := buildCreateOpts(w.Config, w.NodeName, &r.node, r)
			if err := w.Client.CreateVM(ctx, opts); err != nil {
				return fmt.Errorf("create-vm: %w", err)
			}
		case "start-vm":
			if w.Client == nil {
				return errors.New("apply: Client not set")
			}
			if err := w.Client.StartVM(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID); err != nil {
				return fmt.Errorf("start-vm: %w", err)
			}
		default:
			return fmt.Errorf("apply: unknown change kind %q", ch.Kind)
		}
	}

	// Cleanup local ISO (best-effort) to avoid clutter.
	if isoPath != "" {
		_ = os.Remove(isoPath)
	}
	return nil
}

// Verify checks post-apply state.
func (w *SingleVMWorkflow) Verify(ctx context.Context) error {
	r, err := w.resolve()
	if err != nil {
		return err
	}
	if w.Client == nil {
		return errors.New("verify: Client not set")
	}
	vm, err := w.Client.GetVM(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if !strings.EqualFold(vm.Status, "running") {
		return fmt.Errorf("verify: VM %d status=%q, expected running", vm.VMID, vm.Status)
	}
	return nil
}

// Rollback reverses an in-flight Apply. Best-effort destroy.
func (w *SingleVMWorkflow) Rollback(ctx context.Context, _ []Change) error {
	r, err := w.resolve()
	if err != nil {
		return err
	}
	if w.Client == nil {
		return errors.New("rollback: Client not set")
	}
	exists, err := w.Client.VMExists(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	// Stop first (ignore errors), then delete.
	_ = w.Client.StopVM(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID, true)
	return w.Client.DeleteVM(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID, true)
}

// Up is Plan → Apply → Verify (best-effort).
func (w *SingleVMWorkflow) Up(ctx context.Context) error {
	changes, err := w.Plan(ctx)
	if err != nil {
		return err
	}
	if err := w.Apply(ctx, changes); err != nil {
		return err
	}
	// Verify is best-effort — the VM may still be booting when we return.
	if err := w.Verify(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warn: verify: %v\n", err)
	}
	return nil
}

// Down tears the VM down.
func (w *SingleVMWorkflow) Down(ctx context.Context, force bool) error {
	r, err := w.resolve()
	if err != nil {
		return err
	}
	if w.Client == nil {
		return errors.New("down: Client not set")
	}
	exists, err := w.Client.VMExists(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_ = w.Client.StopVM(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID, force)
	return w.Client.DeleteVM(ctx, r.node.Proxmox.NodeName, r.node.Proxmox.VMID, true)
}

// buildCreateOpts maps the env Node into a proxmox.CreateOpts.
func buildCreateOpts(env *config.Env, nodeName string, node *config.Node, r *resolved) proxmox.CreateOpts {
	opts := proxmox.CreateOpts{
		Node:    node.Proxmox.NodeName,
		VMID:    node.Proxmox.VMID,
		Name:    nodeName,
		Tags:    node.Tags,
		OSType:  "l26",
		SCSIHW:  "virtio-scsi-single",
	}
	if node.Resources != nil {
		opts.Memory = node.Resources.Memory
		opts.Cores = node.Resources.Cores
		opts.Sockets = node.Resources.Sockets
		opts.CPU = node.Resources.CPU
		opts.BIOS = node.Resources.BIOS
		opts.Machine = node.Resources.Machine
	}
	if strings.EqualFold(opts.BIOS, "ovmf") {
		opts.EFIDisk = &proxmox.EFIDiskSpec{
			Storage:         firstNonEmpty(storageForDisk(node.Disks), "local-lvm"),
			Format:          "raw",
			PreEnrolledKeys: false,
		}
	}
	// Disks: map config.Disk → proxmox.DiskSpec
	for i, d := range node.Disks {
		iface := d.Interface
		if iface == "" {
			iface = fmt.Sprintf("scsi%d", i)
		}
		opts.Disks = append(opts.Disks, proxmox.DiskSpec{
			Interface: iface,
			Storage:   firstNonEmpty(d.Storage, d.StorageClass, "local-lvm"),
			Size:      d.Size,
			Shared:    d.Shared,
		})
	}
	// NICs
	for i, n := range node.NICs {
		opts.NICs = append(opts.NICs, proxmox.NICSpec{
			Index:  i,
			Bridge: firstNonEmpty(n.Bridge, "vmbr0"),
			MAC:    n.MAC,
			Model:  "virtio",
		})
	}
	// Install ISO (ide2).
	if r != nil && r.iso != nil && r.iso.Image != "" && r.iso.Storage != "" {
		opts.ISOFile = fmt.Sprintf("%s:iso/%s", r.iso.Storage, r.iso.Image)
	}
	_ = env
	return opts
}

// --- helpers -------------------------------------------------------------

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func storageForDisk(disks []config.Disk) string {
	for _, d := range disks {
		if d.Storage != "" {
			return d.Storage
		}
	}
	return ""
}

func safeDistro(ks *config.KickstartConfig) string {
	if ks == nil {
		return "(no-kickstart)"
	}
	return ks.Distro
}
func safeMemory(r *config.Resources) int {
	if r == nil {
		return 0
	}
	return r.Memory
}
func safeCores(r *config.Resources) int {
	if r == nil {
		return 0
	}
	return r.Cores
}
