package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sync"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/metrics"
)

// mcpRequest is the JSON-RPC 2.0 request envelope for MCP.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpToolCall extracts the tool name from tools/call params.
type mcpToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// mcpErrorResponse is a JSON-RPC 2.0 error response.
type mcpErrorResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Error   mcpError    `json:"error"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPProxy intercepts MCP tool calls and checks permissions before forwarding.
type MCPProxy struct {
	client     *daemon.Client
	container  string
	listenAddr string
	upstream   *url.URL // real MCP server
	logger     *metrics.Logger
	httpServer *http.Server

	mu    sync.RWMutex
	cache map[string]daemon.Decision
}

// NewMCPProxy creates a new MCP proxy.
func NewMCPProxy(client *daemon.Client, container, listenAddr string, upstream *url.URL, logger *metrics.Logger) *MCPProxy {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &MCPProxy{
		client:     client,
		container:  container,
		listenAddr: listenAddr,
		upstream:   upstream,
		logger:     logger,
		cache:      make(map[string]daemon.Decision),
	}
}

// Start begins listening and serving. Blocks until stopped.
func (p *MCPProxy) Start() error {
	p.httpServer = &http.Server{
		Addr:    p.listenAddr,
		Handler: p,
	}

	p.logger.Infof("MCP proxy listening on %s, upstream %s", p.listenAddr, p.upstream)

	if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("mcp proxy error: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the proxy.
func (p *MCPProxy) Stop(ctx context.Context) error {
	if p.httpServer != nil {
		p.logger.Infof("Shutting down MCP proxy")
		return p.httpServer.Shutdown(ctx)
	}
	return nil
}

// ServeHTTP intercepts MCP requests, checks permissions for tool calls, and forwards.
func (p *MCPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(mcpErrorResponse{
			JSONRPC: "2.0",
			Error:   mcpError{Code: -32600, Message: "only POST allowed"},
		})
		return
	}

	// Read the body so we can inspect and forward it
	body, err := io.ReadAll(r.Body)
	if err != nil {
		p.writeError(w, nil, -32700, "failed to read request body")
		return
	}

	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		p.writeError(w, nil, -32700, "invalid JSON")
		return
	}

	// Only intercept tools/call — everything else passes through
	if req.Method == "tools/call" {
		var call mcpToolCall
		if err := json.Unmarshal(req.Params, &call); err != nil {
			p.writeError(w, req.ID, -32602, "invalid tool call params")
			return
		}

		decision, err := p.checkPermission(r.Context(), call.Name)
		if err != nil {
			p.logger.Errorf("Permission check failed for MCP tool %s: %v", call.Name, err)
			p.writeError(w, req.ID, -32603, fmt.Sprintf("permission check failed: %v", err))
			return
		}

		if decision != daemon.DecisionAlways && decision != daemon.DecisionOnce {
			p.logger.Infof("Blocked MCP tool call: %s (%s)", call.Name, decision)
			p.writeError(w, req.ID, -32603, fmt.Sprintf("access denied: tool %q is not permitted", call.Name))
			return
		}
	}

	// Forward to upstream MCP server
	p.forward(w, r, body)
}

func (p *MCPProxy) forward(w http.ResponseWriter, originalReq *http.Request, body []byte) {
	upstreamURL := *p.upstream
	upstreamURL.Path = path.Join(upstreamURL.Path, originalReq.URL.Path)

	req, err := http.NewRequestWithContext(originalReq.Context(), http.MethodPost, upstreamURL.String(), bytes.NewReader(body))
	if err != nil {
		p.writeError(w, nil, -32603, "failed to create upstream request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.logger.Errorf("Upstream MCP request failed: %v", err)
		p.writeError(w, nil, -32603, "upstream MCP server unavailable")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// checkPermission checks if an MCP tool is allowed.
func (p *MCPProxy) checkPermission(ctx context.Context, toolName string) (daemon.Decision, error) {
	p.mu.RLock()
	if d, ok := p.cache[toolName]; ok {
		p.mu.RUnlock()
		return d, nil
	}
	p.mu.RUnlock()

	req := daemon.PermissionRequest{
		Container:    p.container,
		ResourceType: daemon.ResourceMCP,
		ResourceID:   toolName,
		Description:  fmt.Sprintf("MCP tool call: %s", toolName),
	}

	resp, err := p.client.Check(ctx, req)
	if err != nil {
		return daemon.DecisionDeny, err
	}

	if resp.Decision == daemon.DecisionAlways || resp.Decision == daemon.DecisionDeny {
		p.mu.Lock()
		p.cache[toolName] = resp.Decision
		p.mu.Unlock()
		return resp.Decision, nil
	}

	resp, err = p.client.Request(ctx, req)
	if err != nil {
		return daemon.DecisionDeny, err
	}

	if resp.Decision == daemon.DecisionAlways || resp.Decision == daemon.DecisionDeny {
		p.mu.Lock()
		p.cache[toolName] = resp.Decision
		p.mu.Unlock()
	}

	return resp.Decision, nil
}

func (p *MCPProxy) writeError(w http.ResponseWriter, id interface{}, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	if code == -32603 {
		w.WriteHeader(http.StatusForbidden)
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}
	json.NewEncoder(w).Encode(mcpErrorResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   mcpError{Code: code, Message: message},
	})
}

// StartMCPProxyOnListener starts the proxy on a pre-created listener (for tests).
func (p *MCPProxy) StartOnListener(ln net.Listener) error {
	p.httpServer = &http.Server{Handler: p}
	return p.httpServer.Serve(ln)
}
