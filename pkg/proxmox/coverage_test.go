package proxmox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var osWriteFile = os.WriteFile

// --- Do() branches --------------------------------------------------------

// JSON body path: any type other than nil / url.Values / *formBody.
func TestDo_JSONBody(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"data":null}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	payload := map[string]string{"a": "b"}
	err := c.Do(context.Background(), http.MethodPost, "/p", payload, nil)
	require.NoError(t, err)
	assert.Equal(t, "application/json", gotCT)
	assert.Contains(t, gotBody, `"a":"b"`)
}

// Unmarshallable body triggers the json.Marshal error path.
func TestDo_JSONMarshalError(t *testing.T) {
	c, _ := NewClient(ClientOpts{Endpoint: "https://x", TokenID: "a", TokenSecret: "b"})
	// channel values cannot be JSON-marshalled.
	err := c.Do(context.Background(), http.MethodPost, "/p", make(chan int), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal body")
}

func TestDo_NetworkError(t *testing.T) {
	// Bogus URL (unresolvable) triggers http.Do error.
	c, _ := NewClient(ClientOpts{
		Endpoint:    "http://127.0.0.1:1",
		TokenID:     "id",
		TokenSecret: "s",
		Timeout:     100 * time.Millisecond,
	})
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.Error(t, err)
}

func TestDo_BadRequestCtx(t *testing.T) {
	c, _ := NewClient(ClientOpts{Endpoint: "http://x", TokenID: "a", TokenSecret: "b"})
	// An invalid method triggers NewRequestWithContext error.
	err := c.Do(context.Background(), "BAD\tMETHOD", "/x", nil, nil)
	require.Error(t, err)
}

func TestDo_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{not valid json`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	var out struct{ X int }
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, &out)
	require.Error(t, err)
}

func TestDo_DataDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data": "string-not-object"}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	var out struct{ X int }
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, &out)
	require.Error(t, err)
}

func TestDo_EmptyDataNoOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data": null}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	// out non-nil but data is null — should not error and out stays zero.
	var out struct{ X int }
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, &out)
	require.NoError(t, err)
	assert.Equal(t, 0, out.X)
}

func TestDo_PathWithoutSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"data":null}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	err := c.Do(context.Background(), http.MethodGet, "noSlash", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "/api2/json/noSlash", gotPath)
}

func TestDo_NonJSONErrorBody(t *testing.T) {
	// parseAPIError: non-JSON body should become Message.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `plain error text`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.Error(t, err)
	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, "plain error text", apiErr.Message)
	assert.Contains(t, apiErr.Error(), "message=")
}

func TestDo_EmptyErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.Error(t, err)
	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
}

// --- ListNodes error path ------------------------------------------------

func TestListNodes_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errors":{"auth":"bad token"}}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	_, err := c.ListNodes(context.Background())
	require.Error(t, err)
}

// --- WaitForTask: context cancellation ------------------------------------

func TestWaitForTask_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"status":"running"}}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := c.WaitForTask(ctx, "pve", "UPID:x", 10*time.Millisecond)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context"))
}

func TestWaitForTask_PollError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	err := c.WaitForTask(context.Background(), "pve", "UPID:x", time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "poll task")
}

// --- APIError.Error() format branches ------------------------------------

func TestAPIError_Format(t *testing.T) {
	e := &APIError{StatusCode: 400}
	// Bare: no message, no errors.
	s := e.Error()
	assert.Contains(t, s, "status=400")

	e2 := &APIError{StatusCode: 500, Message: "boom"}
	assert.Contains(t, e2.Error(), "message=\"boom\"")

	e3 := &APIError{StatusCode: 500, Errors: map[string]string{"z": "1", "a": "2"}}
	// Keys sorted → a comes before z.
	out := e3.Error()
	assert.Less(t, strings.Index(out, "a="), strings.Index(out, "z="))
}

// --- Snapshot: create with empty description + GET error + rollback miss --

func TestSnapshot_CreateNoDesc_NoVMState(t *testing.T) {
	var body string
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		_, _ = io.WriteString(w, `{"data":"UPID:snap"}`)
	})
	c := testClient(t, f.srv)
	require.NoError(t, c.CreateSnapshot(context.Background(), "pve", 100, "s1", "", false))
	assert.Contains(t, body, "snapname=s1")
	assert.NotContains(t, body, "description=")
	assert.NotContains(t, body, "vmstate=")
}

func TestSnapshot_ListError_NotFound(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":{"vmid":"does not exist"}}`)
	})
	c := testClient(t, f.srv)
	_, err := c.ListSnapshots(context.Background(), "pve", 999)
	require.ErrorIs(t, err, ErrVMNotFound)
}

func TestSnapshot_ListError_Other(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"errors":{"x":"y"}}`)
	})
	c := testClient(t, f.srv)
	_, err := c.ListSnapshots(context.Background(), "pve", 100)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrVMNotFound))
}

func TestSnapshot_RollbackNotFound(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":{"snapname":"does not exist"}}`)
	})
	c := testClient(t, f.srv)
	err := c.RollbackSnapshot(context.Background(), "pve", 100, "nope")
	require.ErrorIs(t, err, ErrSnapshotNotFound)
}

func TestSnapshot_RollbackOtherError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	err := c.RollbackSnapshot(context.Background(), "pve", 100, "s")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrSnapshotNotFound))
}

func TestSnapshot_DeleteOtherError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	err := c.DeleteSnapshot(context.Background(), "pve", 100, "s")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrSnapshotNotFound))
}

// Response with null UPID → no WaitForTask call.
func TestSnapshot_CreateNullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)
	require.NoError(t, c.CreateSnapshot(context.Background(), "pve", 100, "s", "", false))
}

func TestSnapshot_DeleteNullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)
	require.NoError(t, c.DeleteSnapshot(context.Background(), "pve", 100, "s"))
}

func TestSnapshot_RollbackNullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)
	require.NoError(t, c.RollbackSnapshot(context.Background(), "pve", 100, "s"))
}

// --- Storage ListStorage error + UploadISO error paths -------------------

func TestListStorage_Error(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{}`)
	})
	c := testClient(t, f.srv)
	_, err := c.ListStorage(context.Background(), "pve")
	require.Error(t, err)
}

func TestUploadISO_OpenFailsMissing(t *testing.T) {
	c, _ := NewClient(ClientOpts{Endpoint: "https://x", TokenID: "a", TokenSecret: "b"})
	err := c.UploadISO(context.Background(), "pve", "s", "/nonexistent/xyz.iso", "r")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open iso")
}

func TestUploadISO_OtherError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	// Need real file.
	dir := t.TempDir()
	p := dir + "/x.iso"
	require.NoError(t, writeFileBytes(p, []byte("x")))
	c := testClient(t, f.srv)
	err := c.UploadISO(context.Background(), "pve", "s", p, "x.iso")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrStorageNotFound))
}

func TestUploadISO_NullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/upload") {
			_, _ = io.WriteString(w, `{"data":null}`)
		}
	})
	dir := t.TempDir()
	p := dir + "/x.iso"
	require.NoError(t, writeFileBytes(p, []byte("x")))
	c := testClient(t, f.srv)
	require.NoError(t, c.UploadISO(context.Background(), "pve", "s", p, ""))
}

func TestSplitContentEdge(t *testing.T) {
	assert.Nil(t, splitContent(""))
	assert.Equal(t, []string{"a", "b"}, splitContent("a, b, "))
}

func TestBaseNameEdge(t *testing.T) {
	assert.Equal(t, "base", baseName("/a/base/"))
	assert.Equal(t, "bare", baseName("bare"))
}

// --- VM: Validate edge + CreateVM error + misc ---------------------------

func TestValidate_AllBranches(t *testing.T) {
	// VMID zero.
	err := CreateOpts{Node: "pve"}.Validate()
	require.Error(t, err)
	// Name empty.
	err = CreateOpts{Node: "pve", VMID: 1}.Validate()
	require.Error(t, err)
	// Happy minimal.
	err = CreateOpts{Node: "pve", VMID: 1, Name: "n"}.Validate()
	require.NoError(t, err)
	// OVMF + EFI provided.
	err = CreateOpts{Node: "pve", VMID: 1, Name: "n", BIOS: "ovmf", EFIDisk: &EFIDiskSpec{Storage: "s"}}.Validate()
	require.NoError(t, err)
	// Disk missing Storage.
	err = CreateOpts{Node: "pve", VMID: 1, Name: "n", Disks: []DiskSpec{{Interface: "scsi0", Size: "1G"}}}.Validate()
	require.Error(t, err)
	// Disk missing Size.
	err = CreateOpts{Node: "pve", VMID: 1, Name: "n", Disks: []DiskSpec{{Interface: "scsi0", Storage: "s"}}}.Validate()
	require.Error(t, err)
}

func TestCreateVM_InvalidOpts(t *testing.T) {
	c, _ := NewClient(ClientOpts{Endpoint: "https://x", TokenID: "a", TokenSecret: "b"})
	err := c.CreateVM(context.Background(), CreateOpts{})
	require.Error(t, err)
}

func TestCreateVM_NullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)
	err := c.CreateVM(context.Background(), CreateOpts{
		Node: "pve", VMID: 200, Name: "n", Cores: 2, Sockets: 1, Memory: 1024,
	})
	require.NoError(t, err)
}

func TestCreateVM_APIError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"vmid":"in use"}}`)
	})
	c := testClient(t, f.srv)
	err := c.CreateVM(context.Background(), CreateOpts{
		Node: "pve", VMID: 200, Name: "n",
	})
	require.Error(t, err)
}

func TestVMStatus_NullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)
	require.NoError(t, c.StartVM(context.Background(), "pve", 100))
}

func TestVMStatus_APIError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	require.Error(t, c.StartVM(context.Background(), "pve", 100))
}

func TestDeleteVM_NullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)
	require.NoError(t, c.DeleteVM(context.Background(), "pve", 100, false))
}

func TestDeleteVM_OtherError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	err := c.DeleteVM(context.Background(), "pve", 100, false)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrVMNotFound))
}

func TestGetVM_OtherError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	_, err := c.GetVM(context.Background(), "pve", 100)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrVMNotFound))
}

func TestGetVM_MissingVMIDFallsBack(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"name":"x","status":"running"}}`)
	})
	c := testClient(t, f.srv)
	vm, err := c.GetVM(context.Background(), "pve", 42)
	require.NoError(t, err)
	assert.Equal(t, 42, vm.VMID, "missing VMID in response should fall back to request arg")
}

func TestListVMs_Error(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{}`)
	})
	c := testClient(t, f.srv)
	_, err := c.ListVMs(context.Background(), "pve")
	require.Error(t, err)
}

func TestVMExists_Propagates(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	_, err := c.VMExists(context.Background(), "pve", 100)
	require.Error(t, err)
}

// --- splitTags: comma separator, extra whitespace ------------------------

func TestSplitTags_CommaAndSemicolon(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, splitTags("a;b,c"))
	assert.Equal(t, []string{"x"}, splitTags("x"))
	assert.Nil(t, splitTags(""))
	// Empty fragments stripped.
	assert.Equal(t, []string{"a", "b"}, splitTags(" a ; ;b ,, "))
}

// --- isNotFound: all branches -------------------------------------------

func TestIsNotFound_NonAPIError(t *testing.T) {
	assert.False(t, isNotFound(errors.New("random")))
}

func TestIsNotFound_Status404(t *testing.T) {
	assert.True(t, isNotFound(&APIError{StatusCode: 404}))
}

func TestIsNotFound_MessageSubstring(t *testing.T) {
	assert.True(t, isNotFound(&APIError{StatusCode: 500, Message: "VM 9 does not exist"}))
	assert.True(t, isNotFound(&APIError{StatusCode: 500, Message: "no such storage"}))
	assert.False(t, isNotFound(&APIError{StatusCode: 500, Message: "permission denied"}))
}

func TestIsNotFound_ErrorsMap(t *testing.T) {
	assert.True(t, isNotFound(&APIError{StatusCode: 500, Errors: map[string]string{"k": "no such snap"}}))
	assert.False(t, isNotFound(&APIError{StatusCode: 500, Errors: map[string]string{"k": "ok"}}))
}

func TestIsNotFound_500NullData(t *testing.T) {
	// Proxmox quirk: GET /nodes/<n>/qemu/<missing-vmid>/status/current returns
	// HTTP 500 with body `{"data":null}` instead of 404. parseAPIError stuffs
	// the raw body into Message when both Errors and Message are empty, so
	// we must recognize that exact pattern.
	assert.True(t, isNotFound(&APIError{StatusCode: 500, Message: `{"data":null}`}))
	assert.True(t, isNotFound(&APIError{StatusCode: 500, Message: `  {"data":null}  `})) // tolerate whitespace
	// Don't false-positive on a 500 with a real message body.
	assert.False(t, isNotFound(&APIError{StatusCode: 500, Message: `{"data":{"vmid":9}}`}))
	assert.False(t, isNotFound(&APIError{StatusCode: 500, Message: `internal error`}))
	// Don't conflate with 200 + null (success/no-op).
	assert.False(t, isNotFound(&APIError{StatusCode: 200, Message: `{"data":null}`}))
}

// --- Boot: ConfigureFirstBoot failure paths ------------------------------

func TestConfigureFirstBoot_AttachIDE2Fails(t *testing.T) {
	// Server rejects the first (IDE2) config update.
	var calls int
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	err := c.ConfigureFirstBoot(context.Background(), "pve", 100,
		"pve:iso/inst.iso", "pve:iso/ks.iso", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attach install iso")
	assert.Equal(t, 1, calls, "should stop at first failure")
}

func TestConfigureFirstBoot_IDE3Fails(t *testing.T) {
	var calls int
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		calls++
		if calls == 1 {
			// IDE2 OK
			_, _ = io.WriteString(w, `{"data":null}`)
			return
		}
		// IDE3 fail
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	err := c.ConfigureFirstBoot(context.Background(), "pve", 100,
		"pve:iso/inst.iso", "pve:iso/ks.iso", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attach kickstart iso")
}

func TestConfigureFirstBoot_SetBootFails(t *testing.T) {
	var calls int
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		calls++
		if calls <= 2 {
			_, _ = io.WriteString(w, `{"data":null}`)
			return
		}
		// Third call (set boot) fails.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	err := c.ConfigureFirstBoot(context.Background(), "pve", 100,
		"pve:iso/inst.iso", "pve:iso/ks.iso", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set boot order")
}

func TestConfigureFirstBoot_StartFails(t *testing.T) {
	var calls int
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		calls++
		if calls <= 3 {
			_, _ = io.WriteString(w, `{"data":null}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":{"x":"bad"}}`)
	})
	c := testClient(t, f.srv)
	err := c.ConfigureFirstBoot(context.Background(), "pve", 100,
		"pve:iso/inst.iso", "pve:iso/ks.iso", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start vm")
}

// --- configUpdate error paths --------------------------------------------

func TestConfigUpdate_VMNotFound(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":{"vmid":"does not exist"}}`)
	})
	c := testClient(t, f.srv)
	err := c.AttachISOAsCDROM(context.Background(), "pve", 999, "ide2", "x:iso/x.iso")
	require.ErrorIs(t, err, ErrVMNotFound)
}

func TestConfigUpdate_NullUPID(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	c := testClient(t, f.srv)
	require.NoError(t, c.EjectISO(context.Background(), "pve", 100, "ide3"))
}

// --- helpers -------------------------------------------------------------

func writeFileBytes(path string, data []byte) error {
	return osWriteFile(path, data, 0o600)
}
