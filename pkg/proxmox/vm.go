package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// VM is a summary view of a Proxmox VM.
type VM struct {
	VMID   int
	Name   string
	Node   string
	Status string
	Cores  int
	Memory int // MiB
	Tags   []string
	Uptime time.Duration
}

// DiskSpec describes one disk attached to a VM.
//
// It is serialized into a form value like `scsi0=<storage>:<size>[,format=<fmt>,shared=1]`.
type DiskSpec struct {
	Interface string // e.g. "scsi0"
	Storage   string
	Size      string // e.g. "64G"
	Shared    bool
	Format    string // "qcow2" | "raw" — default "raw"
}

// NICSpec describes one virtual NIC attached to a VM.
//
// Serialization: `net<index>=<model>=<MAC>,bridge=<br>[,firewall=1][,tag=<vlan>]`.
// When MAC is empty or "auto", the MAC is omitted and Proxmox auto-generates.
type NICSpec struct {
	Index    int    // 0 → net0
	Bridge   string // e.g. "vmbr0"
	Model    string // "virtio" default
	MAC      string // "auto" or "" → auto
	Firewall bool
	VLAN     int // 0 → untagged
}

// EFIDiskSpec describes the EFI vars disk for OVMF VMs.
type EFIDiskSpec struct {
	Storage         string
	Format          string // "raw" default
	PreEnrolledKeys bool
}

// CreateOpts holds all parameters for CreateVM.
type CreateOpts struct {
	Node        string
	VMID        int
	Name        string
	Memory      int // MiB
	Cores       int
	Sockets     int
	CPU         string // default "host"
	BIOS        string // "seabios" | "ovmf"
	Machine     string // e.g. "q35"
	SCSIHW      string // e.g. "virtio-scsi-single"
	OSType      string // e.g. "l26"
	Tags        []string
	StartAtBoot bool
	Protection  bool
	EFIDisk     *EFIDiskSpec // required when BIOS == "ovmf"
	Disks       []DiskSpec
	NICs        []NICSpec
	ISOFile     string // e.g. "proxmox:iso/OracleLinux-9.iso" — attached on ide2
}

// Validate performs sanity checks before sending the request.
func (o CreateOpts) Validate() error {
	if o.Node == "" {
		return errors.New("CreateOpts.Node required")
	}
	if o.VMID <= 0 {
		return errors.New("CreateOpts.VMID must be > 0")
	}
	if o.Name == "" {
		return errors.New("CreateOpts.Name required")
	}
	if strings.EqualFold(o.BIOS, "ovmf") && o.EFIDisk == nil {
		return errors.New("CreateOpts.EFIDisk required when BIOS=ovmf")
	}
	for _, d := range o.Disks {
		if d.Interface == "" || d.Storage == "" || d.Size == "" {
			return fmt.Errorf("disk %+v: Interface, Storage, Size required", d)
		}
	}
	for _, n := range o.NICs {
		if n.Bridge == "" {
			return fmt.Errorf("nic net%d: Bridge required", n.Index)
		}
	}
	return nil
}

// DiskString formats a DiskSpec as a Proxmox form value.
//
//	<storage>:<size>[,format=<fmt>][,shared=1]
func (d DiskSpec) DiskString() string {
	format := d.Format
	if format == "" {
		format = "raw"
	}
	parts := []string{fmt.Sprintf("%s:%s", d.Storage, d.Size)}
	parts = append(parts, "format="+format)
	if d.Shared {
		parts = append(parts, "shared=1")
	}
	return strings.Join(parts, ",")
}

// NICString formats a NICSpec as a Proxmox form value.
//
//	<model>[=<mac>],bridge=<br>[,firewall=1][,tag=<vlan>]
func (n NICSpec) NICString() string {
	model := n.Model
	if model == "" {
		model = "virtio"
	}
	head := model
	if n.MAC != "" && !strings.EqualFold(n.MAC, "auto") {
		head = model + "=" + n.MAC
	}
	parts := []string{head, "bridge=" + n.Bridge}
	if n.Firewall {
		parts = append(parts, "firewall=1")
	}
	if n.VLAN > 0 {
		parts = append(parts, "tag="+strconv.Itoa(n.VLAN))
	}
	return strings.Join(parts, ",")
}

// EFIDiskString formats an EFIDiskSpec as a Proxmox form value.
//
//	<storage>:1,format=<fmt>,efitype=4m,pre-enrolled-keys=<0|1>
func (e EFIDiskSpec) EFIDiskString() string {
	format := e.Format
	if format == "" {
		format = "raw"
	}
	pre := "0"
	if e.PreEnrolledKeys {
		pre = "1"
	}
	return fmt.Sprintf("%s:1,format=%s,efitype=4m,pre-enrolled-keys=%s", e.Storage, format, pre)
}

// CreateVM creates a new VM on the given node. The call submits form-encoded
// parameters to POST /nodes/{node}/qemu. Returns once the backing task has
// been submitted; the caller is responsible for waiting if needed.
//
// The returned UPID is also polled automatically via WaitForTask.
func (c *Client) CreateVM(ctx context.Context, opts CreateOpts) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	form := url.Values{}
	form.Set("vmid", strconv.Itoa(opts.VMID))
	form.Set("name", opts.Name)
	if opts.Memory > 0 {
		form.Set("memory", strconv.Itoa(opts.Memory))
	}
	if opts.Cores > 0 {
		form.Set("cores", strconv.Itoa(opts.Cores))
	}
	if opts.Sockets > 0 {
		form.Set("sockets", strconv.Itoa(opts.Sockets))
	}
	if opts.CPU != "" {
		form.Set("cpu", opts.CPU)
	} else {
		form.Set("cpu", "host")
	}
	if opts.BIOS != "" {
		form.Set("bios", opts.BIOS)
	}
	if opts.Machine != "" {
		form.Set("machine", opts.Machine)
	}
	if opts.SCSIHW != "" {
		form.Set("scsihw", opts.SCSIHW)
	}
	if opts.OSType != "" {
		form.Set("ostype", opts.OSType)
	}
	if len(opts.Tags) > 0 {
		form.Set("tags", strings.Join(opts.Tags, ";"))
	}
	form.Set("agent", "1")
	if opts.StartAtBoot {
		form.Set("onboot", "1")
	}
	if opts.Protection {
		form.Set("protection", "1")
	}
	if opts.EFIDisk != nil {
		form.Set("efidisk0", opts.EFIDisk.EFIDiskString())
	}
	for _, d := range opts.Disks {
		form.Set(d.Interface, d.DiskString())
	}
	for _, n := range opts.NICs {
		form.Set(fmt.Sprintf("net%d", n.Index), n.NICString())
	}
	if opts.ISOFile != "" {
		form.Set("ide2", opts.ISOFile+",media=cdrom")
	}

	var upid string
	if err := c.Do(ctx, http.MethodPost,
		fmt.Sprintf("/nodes/%s/qemu", opts.Node),
		form, &upid); err != nil {
		return err
	}
	if upid == "" {
		return nil
	}
	return c.WaitForTask(ctx, opts.Node, upid, 0)
}

// StartVM boots a VM.
func (c *Client) StartVM(ctx context.Context, node string, vmid int) error {
	return c.vmStatus(ctx, node, vmid, "start")
}

// RebootVM reboots a running VM.
func (c *Client) RebootVM(ctx context.Context, node string, vmid int) error {
	return c.vmStatus(ctx, node, vmid, "reboot")
}

// StopVM powers a VM off. When force is true a hard stop is issued; otherwise
// an ACPI shutdown is requested.
func (c *Client) StopVM(ctx context.Context, node string, vmid int, force bool) error {
	action := "shutdown"
	if force {
		action = "stop"
	}
	return c.vmStatus(ctx, node, vmid, action)
}

func (c *Client) vmStatus(ctx context.Context, node string, vmid int, action string) error {
	path := fmt.Sprintf("%s/status/%s", c.vmPath(node, vmid), action)
	var upid string
	if err := c.Do(ctx, http.MethodPost, path, url.Values{}, &upid); err != nil {
		return err
	}
	if upid == "" {
		return nil
	}
	return c.WaitForTask(ctx, node, upid, 0)
}

// DeleteVM destroys a VM. When purge is true, referenced backups/replication
// jobs are also purged.
func (c *Client) DeleteVM(ctx context.Context, node string, vmid int, purge bool) error {
	path := c.vmPath(node, vmid)
	if purge {
		path += "?purge=1&destroy-unreferenced-disks=1"
	}
	var upid string
	if err := c.Do(ctx, http.MethodDelete, path, nil, &upid); err != nil {
		if isNotFound(err) {
			return ErrVMNotFound
		}
		return err
	}
	if upid == "" {
		return nil
	}
	return c.WaitForTask(ctx, node, upid, 0)
}

// GetVM returns the current status of a VM.
func (c *Client) GetVM(ctx context.Context, node string, vmid int) (*VM, error) {
	var raw struct {
		VMID   json.Number `json:"vmid"`
		Name   string      `json:"name"`
		Status string      `json:"status"`
		Cores  int         `json:"cpus"`
		Memory int64       `json:"maxmem"`
		Tags   string      `json:"tags"`
		Uptime int64       `json:"uptime"`
	}
	path := fmt.Sprintf("%s/status/current", c.vmPath(node, vmid))
	if err := c.Do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		if isNotFound(err) {
			return nil, ErrVMNotFound
		}
		return nil, err
	}
	id, _ := raw.VMID.Int64()
	vm := &VM{
		VMID:   int(id),
		Name:   raw.Name,
		Node:   node,
		Status: raw.Status,
		Cores:  raw.Cores,
		Memory: int(raw.Memory / (1024 * 1024)),
		Uptime: time.Duration(raw.Uptime) * time.Second,
		Tags:   splitTags(raw.Tags),
	}
	if vm.VMID == 0 {
		vm.VMID = vmid
	}
	return vm, nil
}

// ListVMs lists all VMs on the given node.
func (c *Client) ListVMs(ctx context.Context, node string) ([]VM, error) {
	var raws []struct {
		VMID   json.Number `json:"vmid"`
		Name   string      `json:"name"`
		Status string      `json:"status"`
		Cores  int         `json:"cpus"`
		Memory int64       `json:"maxmem"`
		Tags   string      `json:"tags"`
		Uptime int64       `json:"uptime"`
	}
	path := fmt.Sprintf("/nodes/%s/qemu", node)
	if err := c.Do(ctx, http.MethodGet, path, nil, &raws); err != nil {
		return nil, err
	}
	out := make([]VM, 0, len(raws))
	for _, r := range raws {
		id, _ := r.VMID.Int64()
		out = append(out, VM{
			VMID:   int(id),
			Name:   r.Name,
			Node:   node,
			Status: r.Status,
			Cores:  r.Cores,
			Memory: int(r.Memory / (1024 * 1024)),
			Uptime: time.Duration(r.Uptime) * time.Second,
			Tags:   splitTags(r.Tags),
		})
	}
	return out, nil
}

// VMExists returns whether a VM with the given id is present on the node.
func (c *Client) VMExists(ctx context.Context, node string, vmid int) (bool, error) {
	_, err := c.GetVM(ctx, node, vmid)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrVMNotFound) {
		return false, nil
	}
	return false, err
}

// splitTags splits Proxmox's tag string on ';' and ',' (both are used in practice).
func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ';' || r == ','
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// isNotFound returns true if the error indicates a 404/500 "does not exist".
func isNotFound(err error) bool {
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	if apiErr.StatusCode == http.StatusNotFound {
		return true
	}
	m := strings.ToLower(apiErr.Message)
	if strings.Contains(m, "does not exist") || strings.Contains(m, "no such") {
		return true
	}
	for _, v := range apiErr.Errors {
		lv := strings.ToLower(v)
		if strings.Contains(lv, "does not exist") || strings.Contains(lv, "no such") {
			return true
		}
	}
	return false
}
