package daemon

import (
	"context"

	"github.com/dpopsuev/misbah/metrics"
)

// DirectChecker implements proxy.PermissionChecker in-process.
// Same logic as the daemon HTTP endpoints but without network overhead.
type DirectChecker struct {
	whitelist *WhitelistStore
	prompter  Prompter
	audit     *AuditLogger
	logger    *metrics.Logger
}

// NewDirectChecker creates a new in-process permission checker.
func NewDirectChecker(whitelist *WhitelistStore, prompter Prompter, audit *AuditLogger, logger *metrics.Logger) *DirectChecker {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &DirectChecker{
		whitelist: whitelist,
		prompter:  prompter,
		audit:     audit,
		logger:    logger,
	}
}

// Check performs a fast-path whitelist lookup only. No prompt.
func (dc *DirectChecker) Check(_ context.Context, req PermissionRequest) (PermissionResponse, error) {
	decision, ok := dc.whitelist.Check(req.ResourceType, req.ResourceID)
	if !ok {
		return PermissionResponse{Decision: DecisionUnknown}, nil
	}
	return PermissionResponse{Decision: decision}, nil
}

// Request performs the full flow: whitelist → prompt → persist.
func (dc *DirectChecker) Request(_ context.Context, req PermissionRequest) (PermissionResponse, error) {
	if decision, ok := dc.whitelist.Check(req.ResourceType, req.ResourceID); ok {
		if decision == DecisionAlways || decision == DecisionDeny {
			dc.logger.Debugf("Whitelist hit: %s:%s -> %s", req.ResourceType, req.ResourceID, decision)
			if dc.audit != nil {
				dc.audit.LogDecision(req, decision, "whitelist")
			}
			return PermissionResponse{Decision: decision, Reason: "whitelist"}, nil
		}
	}

	decision, err := dc.prompter.Prompt(&req)
	if err != nil {
		dc.logger.Errorf("Prompter error: %v", err)
		decision = DecisionDeny
	}

	if decision == DecisionAlways || decision == DecisionDeny {
		dc.whitelist.Set(req.ResourceType, req.ResourceID, decision)
		if err := dc.whitelist.Save(); err != nil {
			dc.logger.Errorf("Failed to save whitelist: %v", err)
		}
	}

	if dc.audit != nil {
		dc.audit.LogDecision(req, decision, "user")
	}

	return PermissionResponse{Decision: decision}, nil
}
