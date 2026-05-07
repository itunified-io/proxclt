package workflow

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/itunified-io/proxctl/pkg/proxmox"
)

func TestPickFinalizeIP_Priority(t *testing.T) {
	cases := []struct {
		name      string
		ips       map[string]string
		wantIP    string
		wantLabel string
	}{
		{"empty", map[string]string{}, "", ""},
		{"nil", nil, "", ""},
		{"management wins", map[string]string{"management": "10.0.0.1", "public": "1.1.1.1", "private": "10.10.0.1"}, "10.0.0.1", "management"},
		{"public when no management", map[string]string{"public": "1.1.1.1", "private": "10.10.0.1"}, "1.1.1.1", "public"},
		{"private when no public", map[string]string{"private": "10.10.0.1"}, "10.10.0.1", "private"},
		{"any when no canonical key", map[string]string{"vip": "10.20.0.5", "scan": "10.20.0.6"}, "10.20.0.6", "scan"},
		{"empty value skipped", map[string]string{"management": "", "public": "1.2.3.4"}, "1.2.3.4", "public"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip, label := pickFinalizeIP(tc.ips)
			assert.Equal(t, tc.wantIP, ip)
			assert.Equal(t, tc.wantLabel, label)
		})
	}
}

// fakeSSHConn returns the bytes provided as the SSH banner on first read.
type fakeSSHConn struct {
	banner []byte
	read   int32
}

func (f *fakeSSHConn) Read(b []byte) (int, error) {
	if atomic.AddInt32(&f.read, 1) > 1 {
		return 0, errors.New("eof")
	}
	n := copy(b, f.banner)
	return n, nil
}
func (f *fakeSSHConn) Write(b []byte) (int, error)      { return len(b), nil }
func (f *fakeSSHConn) Close() error                     { return nil }
func (f *fakeSSHConn) LocalAddr() net.Addr              { return nil }
func (f *fakeSSHConn) RemoteAddr() net.Addr             { return nil }
func (f *fakeSSHConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeSSHConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeSSHConn) SetWriteDeadline(time.Time) error { return nil }

func TestWaitForSSHUp_BannerSeen(t *testing.T) {
	calls := int32(0)
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		atomic.AddInt32(&calls, 1)
		return &fakeSSHConn{banner: []byte("SSH-2.0-OpenSSH_9.6\r\n")}, nil
	}
	opts := &FinalizeOptions{
		Timeout:      2 * time.Second,
		PollInterval: 10 * time.Millisecond,
		Dialer:       dialer,
	}
	err := waitForSSHUp(context.Background(), "10.0.0.1", opts)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestWaitForSSHUp_RetriesUntilBanner(t *testing.T) {
	calls := int32(0)
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return nil, errors.New("connection refused")
		}
		return &fakeSSHConn{banner: []byte("SSH-2.0-x")}, nil
	}
	opts := &FinalizeOptions{
		Timeout:      2 * time.Second,
		PollInterval: 5 * time.Millisecond,
		Dialer:       dialer,
	}
	err := waitForSSHUp(context.Background(), "10.0.0.1", opts)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(3))
}

func TestWaitForSSHUp_TimeoutReturnsErr(t *testing.T) {
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}
	opts := &FinalizeOptions{
		Timeout:      80 * time.Millisecond,
		PollInterval: 20 * time.Millisecond,
		Dialer:       dialer,
	}
	err := waitForSSHUp(context.Background(), "10.0.0.1", opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ssh-up wait")
}

func TestWaitForSSHUp_NoIP(t *testing.T) {
	err := waitForSSHUp(context.Background(), "", &FinalizeOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no IP")
}

func TestWaitForSSHUp_SkipMode(t *testing.T) {
	// SkipSSHWait should return nil immediately even with empty IP.
	err := waitForSSHUp(context.Background(), "", &FinalizeOptions{SkipSSHWait: true})
	require.NoError(t, err)
}

func TestWaitForSSHUp_ConnectsButNoBanner(t *testing.T) {
	calls := int32(0)
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		n := atomic.AddInt32(&calls, 1)
		// First few opens return non-SSH garbage so we treat as not-yet-up
		if n < 2 {
			return &fakeSSHConn{banner: []byte("HTTP/1.1 ...")}, nil
		}
		return &fakeSSHConn{banner: []byte("SSH-2.0-x")}, nil
	}
	opts := &FinalizeOptions{
		Timeout:      2 * time.Second,
		PollInterval: 5 * time.Millisecond,
		Dialer:       dialer,
	}
	require.NoError(t, waitForSSHUp(context.Background(), "10.0.0.1", opts))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(2))
}

func TestFinalizeOptions_Defaults(t *testing.T) {
	var o FinalizeOptions
	assert.Equal(t, DefaultFinalizeTimeout, o.timeout())
	assert.Equal(t, DefaultFinalizePollInterval, o.pollInterval())
	assert.Equal(t, 22, o.sshPort())
	assert.NotNil(t, o.dialer())
}

func TestFinalizeOptions_Overrides(t *testing.T) {
	o := FinalizeOptions{Timeout: 5 * time.Minute, PollInterval: time.Second, SSHPort: 2222}
	assert.Equal(t, 5*time.Minute, o.timeout())
	assert.Equal(t, time.Second, o.pollInterval())
	assert.Equal(t, 2222, o.sshPort())
}

// Sanity: confirm SSH banner detection prefix-matches just the literal "SSH-".
func TestSSHBannerPrefix(t *testing.T) {
	assert.True(t, strings.HasPrefix("SSH-2.0-OpenSSH", "SSH-"))
	assert.False(t, strings.HasPrefix("HTTP/1.1", "SSH-"))
}

// TestFinalizeSingle_IssuesExpectedQMSets verifies that FinalizeSingle issues
// (in order) ide3 detach, ide2 detach, and boot-order=order=scsi0 against the
// Proxmox API.
func TestFinalizeSingle_IssuesExpectedQMSets(t *testing.T) {
	var (
		mu       sync.Mutex
		bodies   []url.Values
		paths    []string
		methods  []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API token auth: skip /access/ticket — token-based client doesn't hit it.
		if strings.Contains(r.URL.Path, "/tasks/") {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body, _ := url.ParseQuery(string(raw))
		mu.Lock()
		bodies = append(bodies, body)
		paths = append(paths, r.URL.Path)
		methods = append(methods, r.Method)
		mu.Unlock()
		_, _ = io.WriteString(w, `{"data":null}`)
	}))
	defer srv.Close()

	c, err := proxmox.NewClient(proxmox.ClientOpts{
		Endpoint:    srv.URL,
		TokenID:     "root@pam!test",
		TokenSecret: "secret",
		InsecureTLS: true,
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, FinalizeSingle(context.Background(), c, "pve", 2701))

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, bodies, 3, "expected 3 config writes (ide3 detach, ide2 detach, boot order)")
	for _, m := range methods {
		assert.Equal(t, http.MethodPost, m)
	}
	for _, p := range paths {
		assert.Contains(t, p, "/nodes/pve/qemu/2701/config")
	}
	assert.Equal(t, "ide3", bodies[0].Get("delete"), "first call detaches ide3 (kickstart)")
	assert.Equal(t, "ide2", bodies[1].Get("delete"), "second call detaches ide2 (install)")
	assert.Equal(t, "order=scsi0", bodies[2].Get("boot"), "third call resets boot order to scsi0")
}

func TestFinalizeSingle_NilClient(t *testing.T) {
	err := FinalizeSingle(context.Background(), nil, "pve", 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Client not set")
}

// TestProbeSSHViaSystemNC_PortClosed exercises the nc-based probe path
// against a localhost port that is guaranteed closed. Confirms that:
//   1. The function shells out to `nc` (not net.Dial) on systems where
//      the binary is available.
//   2. A closed port returns a non-nil error containing the nc invocation.
//
// We don't test the success path here because that would require a live
// listening service; integration coverage lives in the lab-up live test.
func TestProbeSSHViaSystemNC_PortClosed(t *testing.T) {
	if _, err := exec.LookPath("nc"); err != nil {
		t.Skip("nc not on PATH — probe path not exercised in this test env")
	}
	// 127.0.0.1:1 is reserved + always closed in standard Linux/macOS.
	err := probeSSHViaSystemNC(context.Background(), "127.0.0.1", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nc -z 127.0.0.1 1")
}

// TestWaitForSSHUp_PrefersDialerWhenInjected confirms that injecting a
// custom Dialer (test fake) bypasses the nc shell-out and uses the legacy
// net.Dial code path. This guards against future refactors silently
// breaking test isolation.
func TestWaitForSSHUp_PrefersDialerWhenInjected(t *testing.T) {
	dialerCalls := 0
	fakeDialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		dialerCalls++
		// Force a banner-success path so the test doesn't time out.
		client, server := net.Pipe()
		go func() { _, _ = server.Write([]byte("SSH-2.0-test\r\n")); _ = server.Close() }()
		return client, nil
	}
	opts := &FinalizeOptions{
		Timeout:      2 * time.Second,
		PollInterval: 50 * time.Millisecond,
		Dialer:       fakeDialer,
	}
	err := waitForSSHUp(context.Background(), "10.0.0.1", opts)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, dialerCalls, 1, "injected Dialer should be invoked at least once")
}
