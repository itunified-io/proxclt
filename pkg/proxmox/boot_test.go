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

func TestSetBootOrder_AddsOrderPrefix(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)

	require.NoError(t, c.SetBootOrder(context.Background(), "pve", 100, "scsi0;ide3"))
	body, err := url.ParseQuery(f.requests[0].Body)
	require.NoError(t, err)
	assert.Equal(t, "order=scsi0;ide3", body.Get("boot"))

	// Already-prefixed form is preserved.
	require.NoError(t, c.SetBootOrder(context.Background(), "pve", 100, "order=scsi0"))
	body2, _ := url.ParseQuery(f.requests[1].Body)
	assert.Equal(t, "order=scsi0", body2.Get("boot"))

	err = c.SetBootOrder(context.Background(), "pve", 100, "")
	require.Error(t, err)
}

func TestAttachAndEjectISO(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)

	require.NoError(t, c.AttachISOAsCDROM(context.Background(), "pve", 100, "ide3", "proxmox:iso/ks.iso"))
	body, _ := url.ParseQuery(f.requests[0].Body)
	assert.Equal(t, "proxmox:iso/ks.iso,media=cdrom", body.Get("ide3"))

	require.NoError(t, c.EjectISO(context.Background(), "pve", 100, "ide3"))
	body, _ = url.ParseQuery(f.requests[1].Body)
	assert.Equal(t, "ide3", body.Get("delete"))

	require.Error(t, c.AttachISOAsCDROM(context.Background(), "pve", 100, "", "x"))
	require.Error(t, c.AttachISOAsCDROM(context.Background(), "pve", 100, "ide2", ""))
	require.Error(t, c.EjectISO(context.Background(), "pve", 100, ""))
}

func TestConfigureFirstBoot_ChainsCalls(t *testing.T) {
	var configPosts, startPosts int
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/config"):
			configPosts++
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/status/start"):
			startPosts++
			_, _ = io.WriteString(w, `{"data":"UPID:start"}`)
		default:
			http.NotFound(w, r)
		}
	})

	c := testClient(t, f.srv)
	err := c.ConfigureFirstBoot(context.Background(), "pve", 100,
		"proxmox:iso/oel9.iso", "proxmox:iso/ks.iso", true)
	require.NoError(t, err)
	assert.Equal(t, 3, configPosts, "expected ide2 + ide3 + boot order = 3 config posts")
	assert.Equal(t, 1, startPosts)

	// Without powerOn: just 3 config posts.
	configPosts, startPosts = 0, 0
	err = c.ConfigureFirstBoot(context.Background(), "pve", 100,
		"proxmox:iso/oel9.iso", "proxmox:iso/ks.iso", false)
	require.NoError(t, err)
	assert.Equal(t, 3, configPosts)
	assert.Equal(t, 0, startPosts)

	// Required args
	require.Error(t, c.ConfigureFirstBoot(context.Background(), "pve", 100, "", "x", false))
	require.Error(t, c.ConfigureFirstBoot(context.Background(), "pve", 100, "x", "", false))
}
