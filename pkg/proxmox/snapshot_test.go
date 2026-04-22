package proxmox

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotCRUD(t *testing.T) {
	var create, rollback, delReq recordedReq
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/snapshot"):
			b, _ := io.ReadAll(r.Body)
			create = recordedReq{Method: r.Method, Path: r.URL.Path, Body: string(b)}
			_, _ = io.WriteString(w, `{"data":"UPID:snap"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/snapshot"):
			_, _ = io.WriteString(w, `{"data":[{"name":"pre","description":"d","snaptime":1700000000,"parent":"","vmstate":1},{"name":"current","description":"you are here"}]}`)
		case strings.Contains(r.URL.Path, "/snapshot/pre/rollback"):
			b, _ := io.ReadAll(r.Body)
			rollback = recordedReq{Method: r.Method, Path: r.URL.Path, Body: string(b)}
			_, _ = io.WriteString(w, `{"data":"UPID:rb"}`)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/snapshot/pre"):
			delReq = recordedReq{Method: r.Method, Path: r.URL.Path}
			_, _ = io.WriteString(w, `{"data":"UPID:del"}`)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/snapshot/nope"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"errors":{"snapname":"does not exist"}}`)
		default:
			http.NotFound(w, r)
		}
	})

	c := testClient(t, f.srv)
	ctx := context.Background()

	require.NoError(t, c.CreateSnapshot(ctx, "pve", 100, "pre", "pre-install", true))
	body, err := url.ParseQuery(create.Body)
	require.NoError(t, err)
	assert.Equal(t, "pre", body.Get("snapname"))
	assert.Equal(t, "pre-install", body.Get("description"))
	assert.Equal(t, "1", body.Get("vmstate"))

	snaps, err := c.ListSnapshots(ctx, "pve", 100)
	require.NoError(t, err)
	require.Len(t, snaps, 1, "current entry should be filtered out")
	assert.Equal(t, "pre", snaps[0].Name)
	assert.True(t, snaps[0].VMState)

	require.NoError(t, c.RollbackSnapshot(ctx, "pve", 100, "pre"))
	assert.Equal(t, http.MethodPost, rollback.Method)

	require.NoError(t, c.DeleteSnapshot(ctx, "pve", 100, "pre"))
	assert.Equal(t, http.MethodDelete, delReq.Method)

	err = c.DeleteSnapshot(ctx, "pve", 100, "nope")
	require.ErrorIs(t, err, ErrSnapshotNotFound)
}
