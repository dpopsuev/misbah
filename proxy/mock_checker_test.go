package proxy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// mockChecker implements PermissionChecker for tests.
// Replaces the full daemon server + client stack.
type mockChecker struct {
	mu        sync.RWMutex
	whitelist map[string]Decision // "network:domain" -> decision
	prompter  func(req *PermissionRequest) (Decision, error)
}

func newMockChecker() *mockChecker {
	return &mockChecker{
		whitelist: make(map[string]Decision),
		prompter:  func(req *PermissionRequest) (Decision, error) { return DecisionDeny, nil },
	}
}

func (m *mockChecker) withWhitelist(resourceType ResourceType, resourceID string, decision Decision) *mockChecker {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.whitelist[fmt.Sprintf("%s:%s", resourceType, resourceID)] = decision
	return m
}

func (m *mockChecker) withPrompter(fn func(req *PermissionRequest) (Decision, error)) *mockChecker {
	m.prompter = fn
	return m
}

func (m *mockChecker) Check(ctx context.Context, req PermissionRequest) (PermissionResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", req.ResourceType, req.ResourceID)
	if d, ok := m.whitelist[key]; ok {
		return PermissionResponse{Decision: d}, nil
	}
	return PermissionResponse{Decision: DecisionUnknown}, nil
}

func (m *mockChecker) Request(ctx context.Context, req PermissionRequest) (PermissionResponse, error) {
	// Check whitelist first
	m.mu.RLock()
	key := fmt.Sprintf("%s:%s", req.ResourceType, req.ResourceID)
	if d, ok := m.whitelist[key]; ok {
		m.mu.RUnlock()
		return PermissionResponse{Decision: d}, nil
	}
	m.mu.RUnlock()

	// Prompt
	decision, err := m.prompter(&req)
	if err != nil {
		return PermissionResponse{Decision: DecisionDeny}, err
	}

	// Persist always/deny
	if decision == DecisionAlways || decision == DecisionDeny {
		m.mu.Lock()
		m.whitelist[key] = decision
		m.mu.Unlock()
	}

	return PermissionResponse{Decision: decision}, nil
}

// countingPrompter wraps a decision and counts calls.
type countingPrompter struct {
	decision Decision
	count    *atomic.Int32
}

func newCountingPrompter(decision Decision) *countingPrompter {
	return &countingPrompter{decision: decision, count: &atomic.Int32{}}
}

func (c *countingPrompter) prompt(req *PermissionRequest) (Decision, error) {
	c.count.Add(1)
	return c.decision, nil
}
