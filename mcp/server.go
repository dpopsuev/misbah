package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
)

// MCP method names.
const (
	MethodInitialize = "initialize"
	MethodListTools  = "tools/list"
	MethodCallTool   = "tools/call"
)

// MCP tool names.
const (
	ToolContainerCreate   = "misbah_container_create"
	ToolContainerValidate = "misbah_container_validate"
	ToolContainerInspect  = "misbah_container_inspect"
)

// Server implements the MCP (Model Context Protocol) server for misbah.
type Server struct {
	logger   *metrics.Logger
	recorder *metrics.MetricsRecorder
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
		logger:   logger,
		recorder: recorder,
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

	var result interface{}
	var err error

	switch req.Method {
	case MethodInitialize:
		result, err = s.handleInitialize(r.Context(), req.Params)
	case MethodListTools:
		result, err = s.handleListTools(r.Context())
	case MethodCallTool:
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
			Name:    "misbah",
			Version: "0.2.0",
		},
	}, nil
}

func (s *Server) handleListTools(ctx context.Context) (interface{}, error) {
	tools := []Tool{
		{
			Name:        ToolContainerCreate,
			Description: "Create a new container specification file",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"spec_path":{"type":"string","description":"Path for the new spec YAML file"},
					"name":{"type":"string","description":"Container name"},
					"command":{"type":"array","items":{"type":"string"},"description":"Command to run (default: /bin/bash)"},
					"image":{"type":"string","description":"OCI image (optional)"}
				},
				"required":["spec_path","name"]
			}`),
		},
		{
			Name:        ToolContainerValidate,
			Description: "Validate a container specification file",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"spec_path":{"type":"string","description":"Path to the container spec YAML"}
				},
				"required":["spec_path"]
			}`),
		},
		{
			Name:        ToolContainerInspect,
			Description: "Load and display a container specification",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"spec_path":{"type":"string","description":"Path to the container spec YAML"}
				},
				"required":["spec_path"]
			}`),
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
	case ToolContainerCreate:
		return s.toolContainerCreate(ctx, call.Arguments)
	case ToolContainerValidate:
		return s.toolContainerValidate(ctx, call.Arguments)
	case ToolContainerInspect:
		return s.toolContainerInspect(ctx, call.Arguments)
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

func (s *Server) toolContainerCreate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	specPath, ok := args["spec_path"].(string)
	if !ok {
		return s.errorResult("spec_path is required"), nil
	}
	name, ok := args["name"].(string)
	if !ok {
		return s.errorResult("name is required"), nil
	}

	command := []string{"/bin/bash"}
	if cmdArg, ok := args["command"].([]interface{}); ok {
		command = make([]string, 0, len(cmdArg))
		for _, c := range cmdArg {
			if str, ok := c.(string); ok {
				command = append(command, str)
			}
		}
	}

	image, _ := args["image"].(string)

	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: name,
		},
		Image: image,
		Process: model.ProcessSpec{
			Command: command,
			Cwd:     "/container/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
			PID:   true,
		},
	}

	if err := spec.Validate(); err != nil {
		return s.errorResult(fmt.Sprintf("Invalid spec: %v", err)), nil
	}

	dir := filepath.Dir(specPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return s.errorResult(fmt.Sprintf("Failed to create directory: %v", err)), nil
		}
	}

	if err := spec.SaveContainerSpec(specPath); err != nil {
		return s.errorResult(fmt.Sprintf("Failed to save spec: %v", err)), nil
	}

	return ToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: fmt.Sprintf("Container spec created at %s", specPath),
		}},
	}, nil
}

func (s *Server) toolContainerValidate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	specPath, ok := args["spec_path"].(string)
	if !ok {
		return s.errorResult("spec_path is required"), nil
	}

	spec, err := model.LoadContainerSpec(specPath)
	if err != nil {
		return s.errorResult(fmt.Sprintf("Failed to load spec: %v", err)), nil
	}

	if err := spec.Validate(); err != nil {
		return s.errorResult(fmt.Sprintf("Validation failed: %v", err)), nil
	}

	return ToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: fmt.Sprintf("Container spec '%s' is valid", spec.Metadata.Name),
		}},
	}, nil
}

func (s *Server) toolContainerInspect(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	specPath, ok := args["spec_path"].(string)
	if !ok {
		return s.errorResult("spec_path is required"), nil
	}

	spec, err := model.LoadContainerSpec(specPath)
	if err != nil {
		return s.errorResult(fmt.Sprintf("Failed to load spec: %v", err)), nil
	}

	result, _ := json.MarshalIndent(spec, "", "  ")

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
