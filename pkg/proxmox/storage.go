package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

// Storage describes one Proxmox storage definition visible to a node.
type Storage struct {
	Name    string
	Type    string
	Shared  bool
	Content []string
}

// ListStorage returns the storages available on the given node.
func (c *Client) ListStorage(ctx context.Context, node string) ([]Storage, error) {
	var raws []struct {
		Name    string `json:"storage"`
		Type    string `json:"type"`
		Shared  int    `json:"shared"`
		Content string `json:"content"`
	}
	path := fmt.Sprintf("/nodes/%s/storage", node)
	if err := c.Do(ctx, http.MethodGet, path, nil, &raws); err != nil {
		return nil, err
	}
	out := make([]Storage, 0, len(raws))
	for _, r := range raws {
		out = append(out, Storage{
			Name:    r.Name,
			Type:    r.Type,
			Shared:  r.Shared == 1,
			Content: splitContent(r.Content),
		})
	}
	return out, nil
}

// StorageContentExists returns true if a volume with the given volid is present
// in the storage's content listing. Used by workflow's `verify-kickstart-iso`
// step (skip-kickstart-build mode) to assert the operator-uploaded ISO is in
// place before VM create.
//
// volid is the Proxmox-style identifier `<storage>:<typedir>/<filename>`,
// e.g. `proxmox:iso/ext3adm1_kickstart.iso`.
func (c *Client) StorageContentExists(ctx context.Context, node, storage, volid string) (bool, error) {
	var raws []struct {
		VolID  string `json:"volid"`
		Format string `json:"format"`
		Size   int64  `json:"size"`
	}
	path := fmt.Sprintf("/nodes/%s/storage/%s/content", node, storage)
	if err := c.Do(ctx, http.MethodGet, path, nil, &raws); err != nil {
		return false, err
	}
	for _, r := range raws {
		if r.VolID == volid {
			return true, nil
		}
	}
	return false, nil
}

// UploadISO uploads the file at localPath to the given storage as an ISO.
// When remoteName is empty, the local file's basename is used.
//
// The upload is submitted as multipart/form-data to
// POST /nodes/{node}/storage/{storage}/upload with fields:
//
//	content=iso
//	filename=<remoteName>
//	<file bytes>
func (c *Client) UploadISO(ctx context.Context, node, storage, localPath, remoteName string) error {
	if localPath == "" {
		return errors.New("proxmox: localPath required")
	}
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("proxmox: open iso: %w", err)
	}
	defer f.Close()

	if remoteName == "" {
		remoteName = baseName(localPath)
	}

	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	if err := mw.WriteField("content", "iso"); err != nil {
		return err
	}
	if err := mw.WriteField("filename", remoteName); err != nil {
		return err
	}
	fw, err := mw.CreateFormFile("filename", remoteName)
	if err != nil {
		return fmt.Errorf("proxmox: form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return fmt.Errorf("proxmox: copy iso: %w", err)
	}
	if err := mw.Close(); err != nil {
		return err
	}

	body := &formBody{buf: buf, contentType: mw.FormDataContentType()}

	var upid string
	path := fmt.Sprintf("/nodes/%s/storage/%s/upload", node, storage)
	if err := c.Do(ctx, http.MethodPost, path, body, &upid); err != nil {
		if isNotFound(err) {
			return ErrStorageNotFound
		}
		return err
	}
	if upid == "" {
		return nil
	}
	return c.WaitForTask(ctx, node, upid, 0)
}

// --- helpers --------------------------------------------------------------

func splitContent(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func baseName(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// compile-time check to guarantee we link encoding/json (used elsewhere too).
var _ = json.Marshal
