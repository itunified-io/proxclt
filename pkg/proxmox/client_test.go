package proxmox

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClient returns a Client pointed at the given httptest server.
func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(ClientOpts{
		Endpoint:    srv.URL,
		TokenID:     "root@pam!test",
		TokenSecret: "secret",
		InsecureTLS: true,
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)
	return c
}

func TestNewClient_RequiredFields(t *testing.T) {
	_, err := NewClient(ClientOpts{})
	require.Error(t, err)

	_, err = NewClient(ClientOpts{Endpoint: "https://x"})
	require.Error(t, err)

	_, err = NewClient(ClientOpts{Endpoint: "https://x", TokenID: "id"})
	require.Error(t, err)

	c, err := NewClient(ClientOpts{
		Endpoint: "https://x:8006/api2/json/", TokenID: "id", TokenSecret: "s",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://x:8006", c.Endpoint(),
		"endpoint should be normalized (no /api2/json suffix, no trailing slash)")
}

func TestDo_AuthHeaderAndEnvelope(t *testing.T) {
	var gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		_, _ = io.WriteString(w, `{"data": {"x": 42}}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	var out struct {
		X int `json:"x"`
	}
	err := c.Do(context.Background(), http.MethodGet, "/foo", nil, &out)
	require.NoError(t, err)
	assert.Equal(t, 42, out.X)
	assert.Equal(t, "PVEAPIToken=root@pam!test=secret", gotAuth)
	assert.Equal(t, "application/json", gotAccept)
}

func TestDo_NonJSONBodyFormEncoded(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"data": null}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	form := url.Values{}
	form.Set("a", "1")
	form.Set("b", "two")
	err := c.Do(context.Background(), http.MethodPost, "/x", form, nil)
	require.NoError(t, err)
	assert.Equal(t, "application/x-www-form-urlencoded", gotCT)
	assert.Contains(t, gotBody, "a=1")
	assert.Contains(t, gotBody, "b=two")
}

func TestDo_APIErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"data": null, "errors": {"vmid": "already in use"}}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.Error(t, err)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "expected *APIError, got %T", err)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "already in use", apiErr.Errors["vmid"])
	assert.Contains(t, apiErr.Error(), "vmid=")
}

func TestDo_EmbeddedErrorOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data": null, "errors": {"bad": "nope"}}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil)
	require.Error(t, err)
	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, "nope", apiErr.Errors["bad"])
}

func TestWaitForTask(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 2 {
			_, _ = io.WriteString(w, `{"data":{"upid":"UPID:1","status":"running"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"upid":"UPID:1","status":"stopped","exitstatus":"OK"}}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	err := c.WaitForTask(context.Background(), "pve", "UPID:1", time.Millisecond)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, hits, 2)
}

func TestWaitForTask_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"upid":"UPID:2","status":"stopped","exitstatus":"boom"}}`)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	err := c.WaitForTask(context.Background(), "pve", "UPID:2", time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestListNodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api2/json/nodes", r.URL.Path)
		_, _ = io.WriteString(w, `{"data":[{"node":"pve","status":"online","type":"node"}]}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	nodes, err := c.ListNodes(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "pve", nodes[0].Name)
}

func TestWaitForTask_EmptyUPID(t *testing.T) {
	c, _ := NewClient(ClientOpts{Endpoint: "https://x", TokenID: "a", TokenSecret: "b"})
	err := c.WaitForTask(context.Background(), "pve", "", time.Millisecond)
	require.Error(t, err)
}

