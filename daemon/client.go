package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/dpopsuev/misbah/metrics"
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

// Close cleans up the client's resources.
func (c *Client) Close() {
	c.httpClient.CloseIdleConnections()
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
