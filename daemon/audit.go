package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dpopsuev/misbah/metrics"
)

// AuditEntry is a single line in the audit log.
type AuditEntry struct {
	Timestamp    string       `json:"timestamp"`
	Container    string       `json:"container"`
	ResourceType ResourceType `json:"resource_type"`
	ResourceID   string       `json:"resource_id"`
	Decision     Decision     `json:"decision"`
	Source       string       `json:"source"`
}

// AuditLogger writes structured audit entries as JSON lines.
type AuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
	closer io.Closer
	logger *metrics.Logger
}

// NewAuditLogger creates a new audit logger writing to the given path.
func NewAuditLogger(path string, logger *metrics.Logger) (*AuditLogger, error) {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}

	return &AuditLogger{
		writer: f,
		closer: f,
		logger: logger,
	}, nil
}

// NewAuditLoggerFromWriter creates an audit logger from an existing writer (for tests).
func NewAuditLoggerFromWriter(w io.Writer, logger *metrics.Logger) *AuditLogger {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &AuditLogger{
		writer: w,
		logger: logger,
	}
}

// LogDecision records a permission decision.
func (a *AuditLogger) LogDecision(req PermissionRequest, decision Decision, source string) {
	entry := AuditEntry{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Container:    req.Container,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Decision:     decision,
		Source:       source,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		a.logger.Errorf("Failed to marshal audit entry: %v", err)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	data = append(data, '\n')
	if _, err := a.writer.Write(data); err != nil {
		a.logger.Errorf("Failed to write audit entry: %v", err)
	}
}

// Close closes the audit logger.
func (a *AuditLogger) Close() error {
	if a.closer != nil {
		return a.closer.Close()
	}
	return nil
}
