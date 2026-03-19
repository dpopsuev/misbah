package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
)

// Client is an HTTP client for the permission daemon over a Unix socket.
type Client struct {
	httpClient *http.Client
	socketPath string
	logger     *metrics.Logger
}

// NewClient creates a daemon client that talks over the given Unix socket.
func NewClient(socketPath string, logger *metrics.Logger) *Client {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
		socketPath: socketPath,
		logger:     logger,
	}
}

// Check performs a fast-path whitelist check (POST /permission/check).
// Returns the decision. On miss, returns DecisionUnknown.
func (c *Client) Check(ctx context.Context, req PermissionRequest) (PermissionResponse, error) {
	return c.post(ctx, "/permission/check", req)
}

// Request performs the full permission flow (POST /permission/request).
// This may block while the user is prompted on the host.
func (c *Client) Request(ctx context.Context, req PermissionRequest) (PermissionResponse, error) {
	return c.post(ctx, "/permission/request", req)
}

// ContainerStart sends a container start request to the daemon.
func (c *Client) ContainerStart(ctx context.Context, spec *model.ContainerSpec) (ContainerStartResponse, error) {
	var resp ContainerStartResponse
	err := c.postAny(ctx, "/container/start", ContainerStartRequest{Spec: spec}, &resp)
	return resp, err
}

// ContainerStop sends a container stop request to the daemon.
func (c *Client) ContainerStop(ctx context.Context, name string, force bool) error {
	var resp ContainerActionResponse
	return c.postAny(ctx, "/container/stop", ContainerStopRequest{Name: name, Force: force}, &resp)
}

// ContainerDestroy sends a container destroy request to the daemon.
func (c *Client) ContainerDestroy(ctx context.Context, name string) error {
	var resp ContainerActionResponse
	return c.postAny(ctx, "/container/destroy", ContainerDestroyRequest{Name: name}, &resp)
}

// WhitelistLoad sends a container spec to the daemon to pre-load its whitelist rules.
func (c *Client) WhitelistLoad(ctx context.Context, spec *model.ContainerSpec) error {
	var resp ContainerActionResponse
	return c.postAny(ctx, "/whitelist/load", ContainerStartRequest{Spec: spec}, &resp)
}

// Close cleans up the client's resources.
func (c *Client) Close() {
	c.httpClient.CloseIdleConnections()
}

// postAny sends any JSON body and decodes the response into dest.
func (c *Client) postAny(ctx context.Context, path string, body interface{}, dest interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost"+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("daemon request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		msg := errResp["error"]
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return fmt.Errorf("daemon: %s", msg)
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, req PermissionRequest) (PermissionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return PermissionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost"+path, bytes.NewReader(body))
	if err != nil {
		return PermissionResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return PermissionResponse{}, fmt.Errorf("daemon request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return PermissionResponse{}, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	var pr PermissionResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return PermissionResponse{}, fmt.Errorf("decode response: %w", err)
	}

	return pr, nil
}
