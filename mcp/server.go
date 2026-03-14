package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jabal/jabal/config"
	"github.com/jabal/jabal/metrics"
	"github.com/jabal/jabal/model"
	"github.com/jabal/jabal/mount"
	"github.com/jabal/jabal/provider"
	"github.com/jabal/jabal/validate"
)

// Server implements the MCP (Model Context Protocol) server for jabal.
type Server struct {
	logger    *metrics.Logger
	recorder  *metrics.MetricsRecorder
	lifecycle *mount.Lifecycle
}

// NewServer creates a new MCP server.
func NewServer(logger *metrics.Logger, recorder *metrics.MetricsRecorder) *Server {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	if recorder == nil {
		recorder = metrics.GetDefaultRecorder()
	}

	return &Server{
		logger:    logger,
		recorder:  recorder,
		lifecycle: mount.NewLifecycle(logger, recorder),
	}
}

// ServeHTTP implements http.Handler for MCP protocol.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "only POST requests allowed")
		return
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Route to appropriate handler
	var result interface{}
	var err error

	switch req.Method {
	case "initialize":
		result, err = s.handleInitialize(r.Context(), req.Params)
	case "tools/list":
		result, err = s.handleListTools(r.Context())
	case "tools/call":
		result, err = s.handleCallTool(r.Context(), req.Params)
	default:
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("unknown method: %s", req.Method))
		return
	}

	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(MCPError{
		JSONRPC: "2.0",
		Error: ErrorDetail{
			Code:    status,
			Message: message,
		},
	})
}

// MCP protocol types

type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result"`
}

type MCPError struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Error   ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Initialize response
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools Tools `json:"tools"`
}

type Tools struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: Capabilities{
			Tools: Tools{
				ListChanged: false,
			},
		},
		ServerInfo: ServerInfo{
			Name:    "jabal",
			Version: "0.1.0",
		},
	}, nil
}

func (s *Server) handleListTools(ctx context.Context) (interface{}, error) {
	tools := []Tool{
		{
			Name:        "jabal_list_workspaces",
			Description: "List all available workspaces",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "jabal_create_workspace",
			Description: "Create a new workspace",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"name":{"type":"string","description":"Workspace name"},
					"description":{"type":"string","description":"Optional description"}
				},
				"required":["name"]
			}`),
		},
		{
			Name:        "jabal_get_workspace",
			Description: "Get workspace details including manifest",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"name":{"type":"string","description":"Workspace name"}
				},
				"required":["name"]
			}`),
		},
		{
			Name:        "jabal_update_manifest",
			Description: "Update workspace manifest",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"name":{"type":"string","description":"Workspace name"},
					"manifest":{"type":"object","description":"Manifest YAML as object"}
				},
				"required":["name","manifest"]
			}`),
		},
		{
			Name:        "jabal_validate_workspace",
			Description: "Validate a workspace manifest",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"name":{"type":"string","description":"Workspace name"}
				},
				"required":["name"]
			}`),
		},
		{
			Name:        "jabal_get_status",
			Description: "Get workspace mount status",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"name":{"type":"string","description":"Workspace name"}
				},
				"required":["name"]
			}`),
		},
		{
			Name:        "jabal_list_providers",
			Description: "List available providers (claude, aider, cursor)",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}

	return map[string]interface{}{
		"tools": tools,
	}, nil
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func (s *Server) handleCallTool(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var call ToolCall
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}

	s.logger.Debugf("MCP tool call: %s", call.Name)

	switch call.Name {
	case "jabal_list_workspaces":
		return s.toolListWorkspaces(ctx, call.Arguments)
	case "jabal_create_workspace":
		return s.toolCreateWorkspace(ctx, call.Arguments)
	case "jabal_get_workspace":
		return s.toolGetWorkspace(ctx, call.Arguments)
	case "jabal_update_manifest":
		return s.toolUpdateManifest(ctx, call.Arguments)
	case "jabal_validate_workspace":
		return s.toolValidateWorkspace(ctx, call.Arguments)
	case "jabal_get_status":
		return s.toolGetStatus(ctx, call.Arguments)
	case "jabal_list_providers":
		return s.toolListProviders(ctx, call.Arguments)
	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

type ToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Server) toolListWorkspaces(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	workspaces, err := config.ListWorkspaces()
	if err != nil {
		return s.errorResult(fmt.Sprintf("Failed to list workspaces: %v", err)), nil
	}

	// Load details for each workspace
	var workspaceDetails []map[string]interface{}
	for _, name := range workspaces {
		manifestPath := config.GetManifestPath(name)
		manifest, err := model.LoadManifest(manifestPath)
		if err != nil {
			s.logger.Warnf("Failed to load manifest for %s: %v", name, err)
			continue
		}

		workspaceDetails = append(workspaceDetails, map[string]interface{}{
			"name":        manifest.Name,
			"description": manifest.Description,
			"sources":     len(manifest.Sources),
			"tags":        manifest.Tags,
		})
	}

	result, _ := json.MarshalIndent(map[string]interface{}{
		"workspaces": workspaceDetails,
		"total":      len(workspaceDetails),
	}, "", "  ")

	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: string(result)}},
	}, nil
}

func (s *Server) toolCreateWorkspace(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok {
		return s.errorResult("name is required"), nil
	}

	description, _ := args["description"].(string)

	// Validate name
	if err := model.ValidateWorkspaceName(name); err != nil {
		return s.errorResult(fmt.Sprintf("Invalid workspace name: %v", err)), nil
	}

	// Check if exists
	if config.WorkspaceExists(name) {
		return s.errorResult(fmt.Sprintf("Workspace '%s' already exists", name)), nil
	}

	// Create directory
	if err := config.EnsureWorkspaceDir(name); err != nil {
		return s.errorResult(fmt.Sprintf("Failed to create workspace directory: %v", err)), nil
	}

	// Create manifest
	manifest := &model.Manifest{
		Name:        name,
		Description: description,
		Sources:     []model.SourceSpec{},
		Providers:   make(map[string]interface{}),
		Tags:        []string{},
	}

	manifestPath := config.GetManifestPath(name)
	if err := manifest.SaveManifest(manifestPath); err != nil {
		return s.errorResult(fmt.Sprintf("Failed to save manifest: %v", err)), nil
	}

	return ToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: fmt.Sprintf("✓ Workspace '%s' created successfully at %s", name, manifestPath),
		}},
	}, nil
}

func (s *Server) toolGetWorkspace(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok {
		return s.errorResult("name is required"), nil
	}

	if !config.WorkspaceExists(name) {
		return s.errorResult(fmt.Sprintf("Workspace '%s' does not exist", name)), nil
	}

	manifestPath := config.GetManifestPath(name)
	manifest, err := model.LoadManifest(manifestPath)
	if err != nil {
		return s.errorResult(fmt.Sprintf("Failed to load manifest: %v", err)), nil
	}

	result, _ := json.MarshalIndent(manifest, "", "  ")

	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: string(result)}},
	}, nil
}

func (s *Server) toolUpdateManifest(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok {
		return s.errorResult("name is required"), nil
	}

	manifestData, ok := args["manifest"].(map[string]interface{})
	if !ok {
		return s.errorResult("manifest is required and must be an object"), nil
	}

	// Convert map to manifest struct
	manifestJSON, err := json.Marshal(manifestData)
	if err != nil {
		return s.errorResult(fmt.Sprintf("Failed to serialize manifest: %v", err)), nil
	}

	var manifest model.Manifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return s.errorResult(fmt.Sprintf("Invalid manifest structure: %v", err)), nil
	}

	// Ensure name matches
	manifest.Name = name

	// Validate
	if err := validate.ValidateManifest(&manifest); err != nil {
		return s.errorResult(fmt.Sprintf("Validation failed: %v", err)), nil
	}

	// Save
	manifestPath := config.GetManifestPath(name)
	if err := manifest.SaveManifest(manifestPath); err != nil {
		return s.errorResult(fmt.Sprintf("Failed to save manifest: %v", err)), nil
	}

	return ToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: fmt.Sprintf("✓ Manifest updated successfully for workspace '%s'", name),
		}},
	}, nil
}

func (s *Server) toolValidateWorkspace(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok {
		return s.errorResult("name is required"), nil
	}

	if !config.WorkspaceExists(name) {
		return s.errorResult(fmt.Sprintf("Workspace '%s' does not exist", name)), nil
	}

	manifestPath := config.GetManifestPath(name)
	if err := validate.ValidateManifestFile(manifestPath); err != nil {
		return s.errorResult(fmt.Sprintf("Validation failed: %v", err)), nil
	}

	return ToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: fmt.Sprintf("✓ Workspace '%s' is valid", name),
		}},
	}, nil
}

func (s *Server) toolGetStatus(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok {
		return s.errorResult("name is required"), nil
	}

	status, err := s.lifecycle.GetStatus(name)
	if err != nil {
		return s.errorResult(fmt.Sprintf("Failed to get status: %v", err)), nil
	}

	result, _ := json.MarshalIndent(status, "", "  ")

	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: string(result)}},
	}, nil
}

func (s *Server) toolListProviders(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	providers := provider.GetProviderInfo()

	result, _ := json.MarshalIndent(map[string]interface{}{
		"providers": providers,
		"total":     len(providers),
	}, "", "  ")

	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: string(result)}},
	}, nil
}

func (s *Server) errorResult(message string) ToolResult {
	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: message}},
		IsError: true,
	}
}
