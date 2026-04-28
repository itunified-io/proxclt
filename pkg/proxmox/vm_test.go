package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskAndNICAndEFIString(t *testing.T) {
	d := DiskSpec{Interface: "scsi0", Storage: "local-lvm", Size: "64G", Shared: true}
	assert.Equal(t, "local-lvm:64G,shared=1", d.DiskString())

	d2 := DiskSpec{Interface: "scsi1", Storage: "ssd", Size: "100G", Format: "qcow2"}
	assert.Equal(t, "ssd:100G,format=qcow2", d2.DiskString())

	n := NICSpec{Index: 0, Bridge: "vmbr0", MAC: "AA:BB:CC:DD:EE:FF", VLAN: 10, Firewall: true}
	assert.Equal(t, "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0,firewall=1,tag=10", n.NICString())

	n2 := NICSpec{Index: 1, Bridge: "vmbr1", MAC: "auto"}
	assert.Equal(t, "virtio=auto,bridge=vmbr1", n2.NICString())

	e := EFIDiskSpec{Storage: "local-lvm", PreEnrolledKeys: true}
	assert.Equal(t, "local-lvm:1,format=raw,efitype=4m,pre-enrolled-keys=1", e.EFIDiskString())
}

func TestCreateOptsValidate(t *testing.T) {
	ok := CreateOpts{Node: "pve", VMID: 100, Name: "vm100"}
	require.NoError(t, ok.Validate())

	bad := CreateOpts{Node: "", VMID: 100, Name: "x"}
	require.Error(t, bad.Validate())

	ovmfNoEFI := CreateOpts{Node: "pve", VMID: 101, Name: "v", BIOS: "ovmf"}
	require.Error(t, ovmfNoEFI.Validate())

	badDisk := CreateOpts{Node: "pve", VMID: 1, Name: "x", Disks: []DiskSpec{{Interface: "scsi0"}}}
	require.Error(t, badDisk.Validate())

	badNIC := CreateOpts{Node: "pve", VMID: 1, Name: "x", NICs: []NICSpec{{Index: 0}}}
	require.Error(t, badNIC.Validate())
}

// fakeServer tracks requests for assertion by tests.
type fakeServer struct {
	srv      *httptest.Server
	requests []recordedReq
}

type recordedReq struct {
	Method string
	Path   string
	Body   string
	CT     string
}

func newFake(t *testing.T, handler http.HandlerFunc) *fakeServer {
	f := &fakeServer{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedReq{
			Method: r.Method, Path: r.URL.Path, Body: string(b), CT: r.Header.Get("Content-Type"),
		})
		r.Body = io.NopCloser(strings.NewReader(string(b)))
		handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestCreateVM_FormFieldsAndTaskPolling(t *testing.T) {
	var (
		createCalls int32
		statusCalls int32
	)
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/qemu"):
			atomic.AddInt32(&createCalls, 1)
			_, _ = io.WriteString(w, `{"data":"UPID:pve:123:create"}`)
		case strings.Contains(r.URL.Path, "/tasks/"):
			atomic.AddInt32(&statusCalls, 1)
			_, _ = io.WriteString(w, `{"data":{"upid":"UPID:pve:123:create","status":"stopped","exitstatus":"OK"}}`)
		default:
			http.NotFound(w, r)
		}
	})

	c := testClient(t, f.srv)
	err := c.CreateVM(context.Background(), CreateOpts{
		Node: "pve", VMID: 200, Name: "oel9",
		Memory: 4096, Cores: 2, Sockets: 1,
		BIOS: "ovmf", Machine: "q35", SCSIHW: "virtio-scsi-single", OSType: "l26",
		Tags:        []string{"oel9", "lab"},
		StartAtBoot: true, Protection: true,
		EFIDisk: &EFIDiskSpec{Storage: "local-lvm", PreEnrolledKeys: false},
		Disks:   []DiskSpec{{Interface: "scsi0", Storage: "local-lvm", Size: "64G"}},
		NICs:    []NICSpec{{Index: 0, Bridge: "vmbr0", MAC: "auto"}},
		ISOFile: "proxmox:iso/oel9.iso",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&createCalls))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&statusCalls), int32(1))

	require.GreaterOrEqual(t, len(f.requests), 1)
	body, err := url.ParseQuery(f.requests[0].Body)
	require.NoError(t, err)
	assert.Equal(t, "200", body.Get("vmid"))
	assert.Equal(t, "oel9", body.Get("name"))
	assert.Equal(t, "4096", body.Get("memory"))
	assert.Equal(t, "host", body.Get("cpu"))
	assert.Equal(t, "ovmf", body.Get("bios"))
	assert.Equal(t, "q35", body.Get("machine"))
	assert.Equal(t, "virtio-scsi-single", body.Get("scsihw"))
	assert.Equal(t, "l26", body.Get("ostype"))
	assert.Equal(t, "oel9,lab", body.Get("tags"))
	assert.Equal(t, "1", body.Get("agent"))
	assert.Equal(t, "1", body.Get("onboot"))
	assert.Equal(t, "1", body.Get("protection"))
	assert.Equal(t, "local-lvm:1,format=raw,efitype=4m,pre-enrolled-keys=0", body.Get("efidisk0"))
	assert.Equal(t, "local-lvm:64G", body.Get("scsi0"))
	assert.Equal(t, "virtio=auto,bridge=vmbr0", body.Get("net0"))
	assert.Equal(t, "proxmox:iso/oel9.iso,media=cdrom", body.Get("ide2"))
	assert.Equal(t, "application/x-www-form-urlencoded", f.requests[0].CT)
}

func TestStartStopRebootVM(t *testing.T) {
	var actions []string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		actions = append(actions, parts[len(parts)-1])
		_, _ = io.WriteString(w, `{"data":"UPID:task"}`)
	})

	c := testClient(t, f.srv)
	ctx := context.Background()
	require.NoError(t, c.StartVM(ctx, "pve", 100))
	require.NoError(t, c.StopVM(ctx, "pve", 100, false))
	require.NoError(t, c.StopVM(ctx, "pve", 100, true))
	require.NoError(t, c.RebootVM(ctx, "pve", 100))

	assert.Equal(t, []string{"start", "shutdown", "stop", "reboot"}, actions)
}

func TestDeleteVM_PurgeAndNotFound(t *testing.T) {
	var deleteURL string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteURL = r.URL.String()
		}
		if strings.Contains(r.URL.Path, "/999") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"errors":{"vmid":"VM 999 does not exist"}}`)
			return
		}
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":"UPID:del"}`)
	})

	c := testClient(t, f.srv)
	ctx := context.Background()
	require.NoError(t, c.DeleteVM(ctx, "pve", 100, true))
	assert.Contains(t, deleteURL, "purge=1")

	err := c.DeleteVM(ctx, "pve", 999, false)
	require.ErrorIs(t, err, ErrVMNotFound)
}

func TestGetVMAndListVMs(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status/current") {
			_, _ = io.WriteString(w, `{"data":{"vmid":"100","name":"oel","status":"running","cpus":2,"maxmem":4294967296,"tags":"lab;oel9","uptime":3600}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/qemu") {
			_, _ = io.WriteString(w, `{"data":[{"vmid":100,"name":"oel","status":"running","cpus":2,"maxmem":4294967296,"tags":"lab","uptime":60},{"vmid":101,"name":"ext","status":"stopped","cpus":4,"maxmem":8589934592,"tags":""}]}`)
			return
		}
		http.NotFound(w, r)
	})

	c := testClient(t, f.srv)
	vm, err := c.GetVM(context.Background(), "pve", 100)
	require.NoError(t, err)
	assert.Equal(t, 100, vm.VMID)
	assert.Equal(t, "oel", vm.Name)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, 4096, vm.Memory)
	assert.Equal(t, []string{"lab", "oel9"}, vm.Tags)

	vms, err := c.ListVMs(context.Background(), "pve")
	require.NoError(t, err)
	require.Len(t, vms, 2)
	assert.Equal(t, 101, vms[1].VMID)
	assert.Equal(t, "pve", vms[1].Node)
}

func TestVMExists(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/100/") {
			_, _ = io.WriteString(w, `{"data":{"vmid":100,"name":"x","status":"running"}}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"errors":{"vmid":"does not exist"}}`)
	})
	c := testClient(t, f.srv)

	ok, err := c.VMExists(context.Background(), "pve", 100)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = c.VMExists(context.Background(), "pve", 999)
	require.NoError(t, err)
	assert.False(t, ok)
}

// demonstrate json decoder compatibility for paths used by list endpoints
func TestListVMs_DataEnvelope(t *testing.T) {
	raw := `{"data":[{"vmid":"7","name":"n","status":"running","cpus":1,"maxmem":1048576}]}`
	var wrap struct {
		Data []struct {
			VMID json.Number `json:"vmid"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &wrap))
	assert.Equal(t, "7", wrap.Data[0].VMID.String())
	_ = fmt.Sprintf
}
