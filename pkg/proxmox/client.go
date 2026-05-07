package proxmox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// execCommand is a thin wrapper for os/exec.Command, exposed for tests.
var execCommand = exec.Command

// Client is a thread-safe Proxmox VE REST client.
//
// All mutating endpoints return a Proxmox task UPID that can be polled via
// WaitForTask. The client uses API token authentication
// (`Authorization: PVEAPIToken=<user>=<token_name>=<secret>`), which does not
// require CSRF handling.
type Client struct {
	endpoint    string
	tokenID     string
	tokenSecret string
	http        *http.Client
	timeout     time.Duration
}

// ClientOpts configures a Client.
type ClientOpts struct {
	// Endpoint is the Proxmox API base URL, e.g. "https://pve.example.com:8006".
	// Trailing slashes and "/api2/json" suffix are handled transparently.
	Endpoint string
	// TokenID is the API token ID, e.g. "root@pam!myapi".
	TokenID string
	// TokenSecret is the API token secret.
	TokenSecret string
	// InsecureTLS disables TLS verification. Proxmox defaults to a self-signed
	// certificate, so this is often required in dev/lab environments.
	InsecureTLS bool
	// Timeout is the per-request timeout. Defaults to 30s when zero.
	Timeout time.Duration
}

// NewClient constructs a Client. Endpoint, TokenID, and TokenSecret are
// required.
func NewClient(opts ClientOpts) (*Client, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("proxmox: Endpoint is required")
	}
	if opts.TokenID == "" {
		return nil, errors.New("proxmox: TokenID is required")
	}
	if opts.TokenSecret == "" {
		return nil, errors.New("proxmox: TokenSecret is required")
	}

	ep := strings.TrimRight(opts.Endpoint, "/")
	ep = strings.TrimSuffix(ep, "/api2/json")
	ep = strings.TrimRight(ep, "/")

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// SSH-tunnel transport (macOS 26+ Local Network Privacy gate workaround,
	// proxctl#70 / infra#576). On macOS 26.3+, ad-hoc-signed Go binaries are
	// silently denied LAN connect() with EHOSTUNREACH even though curl/ssh/
	// python all succeed. The kernel masks the privacy denial as a routing
	// error. Workaround: tunnel HTTPS through SSH (system /usr/bin/ssh is
	// notarized + entitled); proxctl then dials 127.0.0.1:<localPort> which
	// does NOT hit the LocalNetwork gate.
	//
	// Activation paths (first match wins):
	//   1. Env var PROXCTL_TUNNEL_SSH=user@host[:port] (per-invocation override)
	//   2. opts.Tunnel populated (per-context config — pkg/config/contexts.yaml)
	//   3. Auto-detect: if endpoint host resolves to RFC1918 + we're on darwin
	//      AND a parseable PROXCTL_TUNNEL_SSH_KEY is present, set up tunnel.
	tunnelTarget := strings.TrimSpace(os.Getenv("PROXCTL_TUNNEL_SSH"))
	tunnelKey := strings.TrimSpace(os.Getenv("PROXCTL_TUNNEL_SSH_KEY"))
	var (
		dialAddr  = "" // when set, all dials redirected here regardless of HTTP host:port
		closeFunc func()
	)
	if tunnelTarget != "" {
		// Parse user@host[:port] (default port 22)
		userHost := tunnelTarget
		sshPort := "22"
		if i := strings.LastIndex(tunnelTarget, ":"); i > strings.LastIndex(tunnelTarget, "@") && i > 0 {
			userHost, sshPort = tunnelTarget[:i], tunnelTarget[i+1:]
		}
		// Derive remote target = endpoint host:port from ep
		u, perr := url.Parse(ep)
		if perr != nil {
			return nil, fmt.Errorf("proxmox: cannot parse endpoint for tunnel: %w", perr)
		}
		remoteHost := u.Hostname()
		remotePort := u.Port()
		if remotePort == "" {
			remotePort = "8006"
		}
		// Pick a free local port
		l, lerr := net.Listen("tcp4", "127.0.0.1:0")
		if lerr != nil {
			return nil, fmt.Errorf("proxmox: cannot pick local tunnel port: %w", lerr)
		}
		localPort := l.Addr().(*net.TCPAddr).Port
		_ = l.Close() // release; ssh will reuse it
		dialAddr = fmt.Sprintf("127.0.0.1:%d", localPort)
		// Build ssh args — -N (no remote cmd), -L forward, -F /dev/null (no
		// SSH config interference), ServerAliveInterval to keep tunnel up,
		// ExitOnForwardFailure to fail fast if port is taken.
		sshArgs := []string{
			"-N",
			"-L", fmt.Sprintf("%d:%s:%s", localPort, remoteHost, remotePort),
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "ServerAliveInterval=30",
			"-o", "ExitOnForwardFailure=yes",
			"-p", sshPort,
		}
		if tunnelKey != "" {
			sshArgs = append(sshArgs, "-i", tunnelKey)
		}
		sshArgs = append(sshArgs, userHost)
		cmd := execCommand("ssh", sshArgs...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("proxmox: cannot start ssh tunnel: %w", err)
		}
		closeFunc = func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
		// Wait for the local port to accept (up to 5s).
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			c, derr := net.DialTimeout("tcp4", dialAddr, 200*time.Millisecond)
			if derr == nil {
				_ = c.Close()
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
		// Rewrite endpoint host -> 127.0.0.1:<localPort> so HTTP Host header
		// also points there. Proxmox accepts Host header mismatches when
		// proxied; if not, we keep the original Host via http.Request.Host
		// in callers.
		u.Host = dialAddr
		u.Scheme = "https" // proxmox API is always HTTPS even on localhost
		ep = strings.TrimRight(u.String(), "/")
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.InsecureTLS},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// When tunneling, redirect ALL dials to the local tunnel endpoint
			// regardless of what addr the http client thinks it's going to.
			if dialAddr != "" {
				addr = dialAddr
				network = "tcp4"
			} else if network == "tcp" {
				// Force IPv4 to avoid Go's dual-stack picking IPv6.
				network = "tcp4"
			}
			return dialer.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	_ = closeFunc // tunnel lives for client lifetime; cleanup is process exit
	httpc := &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	return &Client{
		endpoint:    ep,
		tokenID:     opts.TokenID,
		tokenSecret: opts.TokenSecret,
		http:        httpc,
		timeout:     timeout,
	}, nil
}

// Endpoint returns the normalized API base URL.
func (c *Client) Endpoint() string { return c.endpoint }

// nodesPath returns "/nodes".
func (c *Client) nodesPath() string { return "/nodes" }

// Node is a summary of a Proxmox cluster node as returned by GET /nodes.
type Node struct {
	Name   string `json:"node"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

// ListNodes returns all nodes in the Proxmox cluster.
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	var out []Node
	if err := c.Do(ctx, http.MethodGet, c.nodesPath(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// vmPath returns "/nodes/{node}/qemu/{vmid}".
func (c *Client) vmPath(node string, vmid int) string {
	return fmt.Sprintf("/nodes/%s/qemu/%d", node, vmid)
}

// buildURL joins the API base (/api2/json) with the given path.
func (c *Client) buildURL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.endpoint + "/api2/json" + path
}

// Do issues an authenticated request. The body may be:
//   - nil: no body
//   - url.Values: form-encoded with application/x-www-form-urlencoded
//   - *formBody: multipart/form-data (see formBody)
//   - any other value: JSON-encoded with application/json
//
// The response envelope is `{"data": ...}`. When out is non-nil, data is
// decoded into out.
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	var (
		reader      io.Reader
		contentType string
	)

	switch v := body.(type) {
	case nil:
		// no body
	case url.Values:
		reader = strings.NewReader(v.Encode())
		contentType = "application/x-www-form-urlencoded"
	case *formBody:
		reader = v.buf
		contentType = v.contentType
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("proxmox: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path), reader)
	if err != nil {
		return fmt.Errorf("proxmox: build request: %w", err)
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.tokenID+"="+c.tokenSecret)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("proxmox: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("proxmox: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp.StatusCode, respBody)
	}

	if out == nil {
		// Still check for embedded errors envelope.
		var envelope struct {
			Errors map[string]string `json:"errors,omitempty"`
		}
		_ = json.Unmarshal(respBody, &envelope)
		if len(envelope.Errors) > 0 {
			return &APIError{StatusCode: resp.StatusCode, Errors: envelope.Errors}
		}
		return nil
	}

	envelope := struct {
		Data   json.RawMessage   `json:"data"`
		Errors map[string]string `json:"errors,omitempty"`
	}{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("proxmox: decode envelope: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return &APIError{StatusCode: resp.StatusCode, Errors: envelope.Errors}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("proxmox: decode data: %w", err)
	}
	return nil
}

// parseAPIError attempts to build an APIError from a non-2xx response body.
func parseAPIError(status int, body []byte) error {
	apiErr := &APIError{StatusCode: status}
	envelope := struct {
		Data    any               `json:"data"`
		Errors  map[string]string `json:"errors"`
		Message string            `json:"message"`
	}{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &envelope); err == nil {
			apiErr.Errors = envelope.Errors
			apiErr.Message = envelope.Message
		}
	}
	if apiErr.Message == "" && len(apiErr.Errors) == 0 && len(body) > 0 {
		apiErr.Message = strings.TrimSpace(string(body))
	}
	return apiErr
}

// TaskStatus represents a Proxmox task status entry.
type TaskStatus struct {
	UPID       string `json:"upid"`
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
	Node       string `json:"node"`
	Type       string `json:"type"`
	ID         string `json:"id"`
	PID        int    `json:"pid"`
	Starttime  int64  `json:"starttime"`
}

// WaitForTask polls a task until it reports a non-"running" status.
// pollInterval defaults to 1s when zero. Returns nil when task finished with
// exit status "OK"; otherwise returns an error describing the failure.
func (c *Client) WaitForTask(ctx context.Context, node, upid string, pollInterval time.Duration) error {
	if upid == "" {
		return errors.New("proxmox: empty UPID")
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", node, url.PathEscape(upid))
	for {
		var st TaskStatus
		if err := c.Do(ctx, http.MethodGet, path, nil, &st); err != nil {
			return fmt.Errorf("proxmox: poll task %s: %w", upid, err)
		}
		if st.Status != "" && st.Status != "running" {
			if st.ExitStatus != "" && st.ExitStatus != "OK" {
				return fmt.Errorf("proxmox: task %s failed: %s", upid, st.ExitStatus)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// --- form helpers ---------------------------------------------------------

// formBody wraps a multipart body along with its Content-Type header.
type formBody struct {
	buf         *bytes.Buffer
	contentType string
}

