package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	osuser "os/user"
	"path/filepath"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/proxy"
)

// VsockConfig holds vsock transport settings for Kata containers.
type VsockConfig struct {
	Port   uint32 // host vsock listen port
	BinDir string // host path to misbah binaries
}

// Server is the unified daemon HTTP server on a Unix socket.
// Handles both permission brokering and container lifecycle (for Kata).
type Server struct {
	whitelist  *WhitelistStore
	prompter   Prompter
	audit      *AuditLogger
	logger     *metrics.Logger
	lifecycle  ContainerLifecycle
	proxyMgr   *ProxyManager
	vsockCfg   *VsockConfig
	listener   net.Listener
	httpServer *http.Server
}

// ServerOption configures optional server capabilities.
type ServerOption func(*Server)

// WithLifecycle enables container lifecycle endpoints (for Kata backend).
func WithLifecycle(lc ContainerLifecycle) ServerOption {
	return func(s *Server) {
		s.lifecycle = lc
	}
}

// WithProxyManager enables per-container proxy management.
func WithProxyManager(pm *ProxyManager) ServerOption {
	return func(s *Server) {
		s.proxyMgr = pm
	}
}

// WithVsockConfig enables vsock transport for Kata containers.
func WithVsockConfig(port uint32, binDir string) ServerOption {
	return func(s *Server) {
		s.vsockCfg = &VsockConfig{Port: port, BinDir: binDir}
	}
}

// NewServer creates a new daemon server.
func NewServer(whitelist *WhitelistStore, prompter Prompter, audit *AuditLogger, logger *metrics.Logger, opts ...ServerOption) *Server {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	s := &Server{
		whitelist: whitelist,
		prompter:  prompter,
		audit:     audit,
		logger:    logger,
	}
	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/permission/request", s.handleRequest)
	mux.HandleFunc("/permission/check", s.handleCheck)
	mux.HandleFunc("/permission/list", s.handleList)
	mux.HandleFunc("/container/start", s.handleContainerStart)
	mux.HandleFunc("/container/stop", s.handleContainerStop)
	mux.HandleFunc("/container/destroy", s.handleContainerDestroy)
	mux.HandleFunc("/container/list", s.handleContainerList)
	mux.HandleFunc("/container/status", s.handleContainerStatus)
	mux.HandleFunc("/container/exec", s.handleContainerExec)
	mux.HandleFunc("/whitelist/load", s.handleWhitelistLoad)
	s.httpServer = &http.Server{Handler: mux}

	return s
}

// Start listens on the given Unix socket path and serves requests.
func (s *Server) Start(socketPath string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove stale socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	s.listener = ln

	// Set socket permissions: root:misbah 660 (like Docker's docker.sock).
	// Users in the 'misbah' group can connect. Others cannot.
	if err := setSocketGroupPermissions(socketPath, "misbah", s.logger); err != nil {
		s.logger.Warnf("Socket group setup: %v (falling back to root-only access)", err)
	}

	s.logger.Infof("Permission daemon listening on %s", socketPath)

	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	if s.proxyMgr != nil {
		s.proxyMgr.StopAll()
	}
	if s.httpServer != nil {
		s.logger.Infof("Shutting down permission daemon")
		return s.httpServer.Shutdown(context.Background())
	}
	return nil
}

// handleRequest is the full permission flow: whitelist check -> prompt -> persist.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req PermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Check whitelist for ALWAYS or DENY
	if decision, ok := s.whitelist.Check(req.ResourceType, req.ResourceID); ok {
		if decision == DecisionAlways || decision == DecisionDeny {
			s.logger.Debugf("Whitelist hit: %s:%s -> %s", req.ResourceType, req.ResourceID, decision)
			if s.audit != nil {
				s.audit.LogDecision(req, decision, "whitelist")
			}
			s.writeJSON(w, PermissionResponse{Decision: decision, Reason: "whitelist"})
			return
		}
	}

	// Prompt user
	decision, err := s.prompter.Prompt(&req)
	if err != nil {
		s.logger.Errorf("Prompter error: %v", err)
		decision = DecisionDeny
	}

	// Persist ALWAYS and DENY decisions
	if decision == DecisionAlways || decision == DecisionDeny {
		s.whitelist.Set(req.ResourceType, req.ResourceID, decision)
		if err := s.whitelist.Save(); err != nil {
			s.logger.Errorf("Failed to save whitelist: %v", err)
		}
	}

	if s.audit != nil {
		s.audit.LogDecision(req, decision, "user")
	}

	s.writeJSON(w, PermissionResponse{Decision: decision})
}

// handleCheck is the fast path: whitelist lookup only, no prompt.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req PermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	decision, ok := s.whitelist.Check(req.ResourceType, req.ResourceID)
	if !ok {
		s.writeJSON(w, PermissionResponse{Decision: DecisionUnknown})
		return
	}

	s.writeJSON(w, PermissionResponse{Decision: decision})
}

// handleList returns all whitelist rules.
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	rules := s.whitelist.Rules()

	// Convert to string map for JSON
	out := make(map[string]string, len(rules))
	for k, v := range rules {
		out[k] = string(v)
	}

	s.writeJSON(w, map[string]interface{}{"rules": out})
}

func (s *Server) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// setSocketGroupPermissions sets the socket to group-readable/writable for the given group.
func setSocketGroupPermissions(socketPath, groupName string, logger *metrics.Logger) error {
	grp, err := osuser.LookupGroup(groupName)
	if err != nil {
		return fmt.Errorf("group %q not found: %w (create with: sudo groupadd %s)", groupName, err, groupName)
	}

	gid := 0
	if _, err := fmt.Sscanf(grp.Gid, "%d", &gid); err != nil {
		return fmt.Errorf("invalid GID for group %s: %w", groupName, err)
	}

	if err := os.Chown(socketPath, 0, gid); err != nil {
		return fmt.Errorf("failed to chown socket to root:%s: %w", groupName, err)
	}

	if err := os.Chmod(socketPath, 0660); err != nil {
		return fmt.Errorf("failed to chmod socket: %w", err)
	}

	logger.Infof("Socket permissions set: root:%s 660", groupName)
	return nil
}

// handleWhitelistLoad loads whitelist rules from a container spec.
func (s *Server) handleWhitelistLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req ContainerStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if req.Spec != nil {
		s.whitelist.LoadFromSpec(req.Spec)
		if err := s.whitelist.Save(); err != nil {
			s.logger.Warnf("Failed to persist whitelist: %v", err)
		}
	}

	s.writeJSON(w, ContainerActionResponse{Status: "loaded"})
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// handleContainerStart starts a container via the lifecycle manager.
func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	if s.lifecycle == nil {
		s.writeError(w, http.StatusServiceUnavailable, "container lifecycle not available (CRI backend not configured)")
		return
	}

	var req ContainerStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if req.Spec == nil {
		s.writeError(w, http.StatusBadRequest, "spec is required")
		return
	}

	// Load spec whitelist into daemon's whitelist store
	s.whitelist.LoadFromSpec(req.Spec)

	name := req.Spec.Metadata.Name
	s.logger.Infof("Starting container via daemon: %s", name)

	// Start proxy for this container (if proxy manager configured)
	if s.proxyMgr != nil {
		proxyAddr, proxyErr := s.proxyMgr.Start(name)
		if proxyErr != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("proxy start failed: %v", proxyErr))
			return
		}

		if req.Spec.Runtime == model.RuntimeKata && s.vsockCfg != nil {
			// Kata path: proxy env vars point to VM-local forwarder,
			// vsock forwarder config is injected into the CRI container config.
			forwarderAddr := fmt.Sprintf("127.0.0.1:%d", s.vsockCfg.Port)
			req.Spec.Process.Env = append(req.Spec.Process.Env, proxy.ProxyEnvVars(forwarderAddr, "")...)
			req.Spec.VsockForwarder = &model.VsockForwarderConfig{
				Port:   s.vsockCfg.Port,
				BinDir: s.vsockCfg.BinDir,
			}
		} else {
			// Namespace path: proxy env vars point directly to host proxy
			socketPath := s.listener.Addr().String()
			req.Spec.Process.Env = append(req.Spec.Process.Env, proxy.ProxyEnvVars(proxyAddr, socketPath)...)
		}
	}

	// Start in a goroutine — lifecycle.Start blocks until the container exits
	go func() {
		if err := s.lifecycle.Start(req.Spec); err != nil {
			s.logger.Errorf("Container %s failed: %v", name, err)
		} else {
			s.logger.Infof("Container %s exited successfully", name)
		}
		if s.proxyMgr != nil {
			s.proxyMgr.Stop(name)
		}
	}()

	s.writeJSON(w, ContainerStartResponse{
		ID:     name,
		Status: "started",
	})
}

// handleContainerStop stops a running container.
func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	if s.lifecycle == nil {
		s.writeError(w, http.StatusServiceUnavailable, "container lifecycle not available")
		return
	}

	var req ContainerStopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	s.logger.Infof("Stopping container via daemon: %s (force=%v)", req.Name, req.Force)

	if err := s.lifecycle.Stop(req.Name, req.Force); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("stop failed: %v", err))
		return
	}

	s.writeJSON(w, ContainerActionResponse{Status: "stopped"})
}

// handleContainerDestroy destroys a container and cleans up resources.
func (s *Server) handleContainerDestroy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	if s.lifecycle == nil {
		s.writeError(w, http.StatusServiceUnavailable, "container lifecycle not available")
		return
	}

	var req ContainerDestroyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	s.logger.Infof("Destroying container via daemon: %s", req.Name)

	if err := s.lifecycle.Destroy(req.Name); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("destroy failed: %v", err))
		return
	}

	s.writeJSON(w, ContainerActionResponse{Status: "destroyed"})
}

// handleContainerList returns all managed containers.
func (s *Server) handleContainerList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	if s.lifecycle == nil {
		s.writeError(w, http.StatusServiceUnavailable, "container lifecycle not available")
		return
	}

	infos, err := s.lifecycle.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("list failed: %v", err))
		return
	}

	s.writeJSON(w, ContainerListResponse{Containers: infos})
}

// handleContainerStatus returns the status of a single container.
func (s *Server) handleContainerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	if s.lifecycle == nil {
		s.writeError(w, http.StatusServiceUnavailable, "container lifecycle not available")
		return
	}

	var req ContainerStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	info, err := s.lifecycle.Status(req.Name)
	if err != nil {
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("status failed: %v", err))
		return
	}

	s.writeJSON(w, info)
}

// handleContainerExec executes a command in a running container.
func (s *Server) handleContainerExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	if s.lifecycle == nil {
		s.writeError(w, http.StatusServiceUnavailable, "container lifecycle not available")
		return
	}

	var req ContainerExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	stdout, stderr, exitCode, err := s.lifecycle.Exec(req.Name, req.Command, req.Timeout)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("exec failed: %v", err))
		return
	}

	s.writeJSON(w, ContainerExecResponse{
		ExitCode: exitCode,
		Stdout:   string(stdout),
		Stderr:   string(stderr),
	})
}
