package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AttachISOAsCDROM attaches the given ISO spec (e.g. "proxmox:iso/foo.iso")
// to the specified IDE port (e.g. "ide2" or "ide3") as a cdrom.
func (c *Client) AttachISOAsCDROM(ctx context.Context, node string, vmid int, idePort, iso string) error {
	if idePort == "" {
		return errors.New("proxmox: idePort required (e.g. ide2)")
	}
	if iso == "" {
		return errors.New("proxmox: iso spec required")
	}
	form := url.Values{}
	form.Set(idePort, iso+",media=cdrom")
	return c.configUpdate(ctx, node, vmid, form)
}

// EjectISO removes the ISO from the specified IDE slot.
func (c *Client) EjectISO(ctx context.Context, node string, vmid int, idePort string) error {
	if idePort == "" {
		return errors.New("proxmox: idePort required")
	}
	form := url.Values{}
	form.Set("delete", idePort)
	return c.configUpdate(ctx, node, vmid, form)
}

// SetBootOrder sets the boot device priority. boot is Proxmox's order= value,
// a comma or semicolon-separated list like "scsi0;ide3;net0" or "order=scsi0;ide3".
//
// The value is normalized: callers may pass either "scsi0;ide3" or
// "order=scsi0;ide3" — both produce `boot=order=scsi0;ide3`.
func (c *Client) SetBootOrder(ctx context.Context, node string, vmid int, boot string) error {
	boot = strings.TrimSpace(boot)
	if boot == "" {
		return errors.New("proxmox: boot order required")
	}
	if !strings.HasPrefix(boot, "order=") {
		boot = "order=" + boot
	}
	form := url.Values{}
	form.Set("boot", boot)
	return c.configUpdate(ctx, node, vmid, form)
}

// ConfigureFirstBoot prepares a VM for automated install: attaches the OS
// install ISO on ide2, the kickstart / autoinstall ISO on ide3, and sets
// the boot order to prefer the primary disk then the kickstart ISO then the
// install ISO. When powerOn is true, the VM is started at the end.
//
// This mirrors proxmoxManager._configure_first_boot in the orcl_automization
// reference: three sequential config calls followed by an optional start.
func (c *Client) ConfigureFirstBoot(ctx context.Context, node string, vmid int, installISO, kickstartISO string, powerOn bool) error {
	if installISO == "" || kickstartISO == "" {
		return errors.New("proxmox: installISO and kickstartISO required")
	}
	if err := c.AttachISOAsCDROM(ctx, node, vmid, "ide2", installISO); err != nil {
		return fmt.Errorf("attach install iso: %w", err)
	}
	if err := c.AttachISOAsCDROM(ctx, node, vmid, "ide3", kickstartISO); err != nil {
		return fmt.Errorf("attach kickstart iso: %w", err)
	}
	if err := c.SetBootOrder(ctx, node, vmid, "scsi0;ide3;ide2"); err != nil {
		return fmt.Errorf("set boot order: %w", err)
	}
	if powerOn {
		if err := c.StartVM(ctx, node, vmid); err != nil {
			return fmt.Errorf("start vm: %w", err)
		}
	}
	return nil
}

// configUpdate posts a config-update request. Config updates return either
// an UPID or null depending on the change; we handle both.
func (c *Client) configUpdate(ctx context.Context, node string, vmid int, form url.Values) error {
	path := fmt.Sprintf("%s/config", c.vmPath(node, vmid))
	var upid string
	if err := c.Do(ctx, http.MethodPost, path, form, &upid); err != nil {
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
