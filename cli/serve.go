package cli

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dpopsuev/misbah/mcp"
	"github.com/spf13/cobra"
)

var (
	serveAddr string
	servePort int
)

// serveCmd represents the serve command.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server",
	Long: `Start the misbah MCP (Model Context Protocol) server.

The MCP server exposes misbah functionality via HTTP JSON-RPC for integration
with AI agents and automation tools.

Available MCP tools:
  - misbah_list_workspaces: List all workspaces
  - misbah_create_workspace: Create a new workspace
  - misbah_get_workspace: Get workspace details
  - misbah_update_manifest: Update workspace manifest
  - misbah_validate_workspace: Validate workspace
  - misbah_get_status: Get workspace mount status
  - misbah_list_providers: List available providers

Examples:
  misbah serve
  misbah serve --port 8090
  misbah serve --addr 0.0.0.0 --port 8080`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", "127.0.0.1", "Server address to bind to")
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Server port")
}

func runServe(cmd *cobra.Command, args []string) error {
	addr := fmt.Sprintf("%s:%d", serveAddr, servePort)

	logger.Infof("Starting misbah MCP server on %s", addr)

	// Create MCP server
	server := mcp.NewServer(logger, recorder)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", server)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Infof("Shutting down MCP server...")
		httpServer.Close()
	}()

	logger.Infof("MCP server started successfully")
	logger.Infof("Listening on http://%s", addr)
	logger.Infof("Protocol: MCP 2024-11-05")
	logger.Infof("")
	logger.Infof("Test with:")
	logger.Infof("  curl -X POST http://%s -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}'", addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	logger.Infof("MCP server stopped")
	return nil
}
