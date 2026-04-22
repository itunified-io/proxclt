package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Snapshot is a qemu VM snapshot descriptor.
type Snapshot struct {
	Name        string
	Description string
	SnapTime    time.Time
	Parent      string
	VMState     bool
}

// CreateSnapshot creates a new snapshot of the given VM. When vmstate is true,
// the running VM's RAM is captured as well.
func (c *Client) CreateSnapshot(ctx context.Context, node string, vmid int, name, description string, vmstate bool) error {
	form := url.Values{}
	form.Set("snapname", name)
	if description != "" {
		form.Set("description", description)
	}
	if vmstate {
		form.Set("vmstate", "1")
	}
	path := fmt.Sprintf("%s/snapshot", c.vmPath(node, vmid))
	var upid string
	if err := c.Do(ctx, http.MethodPost, path, form, &upid); err != nil {
		return err
	}
	if upid == "" {
		return nil
	}
	return c.WaitForTask(ctx, node, upid, 0)
}

// ListSnapshots returns all snapshots for the given VM.
//
// Note: Proxmox includes a synthetic "current" entry in the list; it is
// filtered out of the returned slice.
func (c *Client) ListSnapshots(ctx context.Context, node string, vmid int) ([]Snapshot, error) {
	var raws []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		SnapTime    int64  `json:"snaptime"`
		Parent      string `json:"parent"`
		VMState     int    `json:"vmstate"`
	}
	path := fmt.Sprintf("%s/snapshot", c.vmPath(node, vmid))
	if err := c.Do(ctx, http.MethodGet, path, nil, &raws); err != nil {
		if isNotFound(err) {
			return nil, ErrVMNotFound
		}
		return nil, err
	}
	out := make([]Snapshot, 0, len(raws))
	for _, r := range raws {
		if r.Name == "current" {
			continue
		}
		out = append(out, Snapshot{
			Name:        r.Name,
			Description: r.Description,
			SnapTime:    time.Unix(r.SnapTime, 0).UTC(),
			Parent:      r.Parent,
			VMState:     r.VMState == 1,
		})
	}
	return out, nil
}

// DeleteSnapshot removes a snapshot by name.
func (c *Client) DeleteSnapshot(ctx context.Context, node string, vmid int, name string) error {
	path := fmt.Sprintf("%s/snapshot/%s", c.vmPath(node, vmid), name)
	var upid string
	if err := c.Do(ctx, http.MethodDelete, path, nil, &upid); err != nil {
		if isNotFound(err) {
			return ErrSnapshotNotFound
		}
		return err
	}
	if upid == "" {
		return nil
	}
	return c.WaitForTask(ctx, node, upid, 0)
}

// RollbackSnapshot reverts a VM to the state captured by the named snapshot.
func (c *Client) RollbackSnapshot(ctx context.Context, node string, vmid int, name string) error {
	path := fmt.Sprintf("%s/snapshot/%s/rollback", c.vmPath(node, vmid), name)
	var upid string
	if err := c.Do(ctx, http.MethodPost, path, url.Values{}, &upid); err != nil {
		if isNotFound(err) {
			return ErrSnapshotNotFound
		}
		return err
	}
	if upid == "" {
		return nil
	}
	return c.WaitForTask(ctx, node, upid, 0)
}
