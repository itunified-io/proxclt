package proxmox

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStorage(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"storage":"local","type":"dir","shared":0,"content":"iso,backup"},{"storage":"proxmox","type":"nfs","shared":1,"content":"iso,images"}]}`)
	})
	c := testClient(t, f.srv)
	out, err := c.ListStorage(context.Background(), "pve")
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "local", out[0].Name)
	assert.False(t, out[0].Shared)
	assert.Equal(t, []string{"iso", "backup"}, out[0].Content)
	assert.True(t, out[1].Shared)
}

func TestUploadISO_Multipart(t *testing.T) {
	// create a temporary ISO (fake content)
	dir := t.TempDir()
	iso := filepath.Join(dir, "ext5adm1_kickstart.iso")
	require.NoError(t, os.WriteFile(iso, []byte("FAKEISO-BYTES"), 0o600))

	var gotContent, gotFilename, gotFileBody, gotCT string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		gotCT = r.Header.Get("Content-Type")
		mt, params, err := mime.ParseMediaType(gotCT)
		require.NoError(t, err)
		require.Equal(t, "multipart/form-data", mt)
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			body, _ := io.ReadAll(p)
			switch p.FormName() {
			case "content":
				gotContent = string(body)
			case "filename":
				if p.FileName() != "" {
					gotFileBody = string(body)
				} else {
					gotFilename = string(body)
				}
			}
		}
		_, _ = io.WriteString(w, `{"data":"UPID:upload"}`)
	})

	c := testClient(t, f.srv)
	err := c.UploadISO(context.Background(), "pve", "proxmox", iso, "")
	require.NoError(t, err)
	assert.Equal(t, "iso", gotContent)
	assert.Equal(t, "ext5adm1_kickstart.iso", gotFilename)
	assert.Equal(t, "FAKEISO-BYTES", gotFileBody)
	assert.True(t, strings.HasPrefix(gotCT, "multipart/form-data"))
}

func TestUploadISO_StorageNotFound(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "x.iso")
	require.NoError(t, os.WriteFile(iso, []byte("x"), 0o600))

	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":{"storage":"does not exist"}}`)
	})
	c := testClient(t, f.srv)
	err := c.UploadISO(context.Background(), "pve", "missing", iso, "x.iso")
	require.ErrorIs(t, err, ErrStorageNotFound)
}

func TestUploadISO_LocalPathRequired(t *testing.T) {
	c, _ := NewClient(ClientOpts{Endpoint: "https://x", TokenID: "a", TokenSecret: "b"})
	err := c.UploadISO(context.Background(), "pve", "s", "", "r")
	require.Error(t, err)
}
