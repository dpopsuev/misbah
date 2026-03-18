package daemon

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalPrompterParsesInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Decision
	}{
		{"lowercase o", "o\n", DecisionOnce},
		{"uppercase O", "O\n", DecisionOnce},
		{"word once", "once\n", DecisionOnce},
		{"word ONCE", "ONCE\n", DecisionOnce},
		{"lowercase a", "a\n", DecisionAlways},
		{"uppercase A", "A\n", DecisionAlways},
		{"word always", "always\n", DecisionAlways},
		{"lowercase d", "d\n", DecisionDeny},
		{"uppercase D", "D\n", DecisionDeny},
		{"word deny", "deny\n", DecisionDeny},
		{"with whitespace", "  o  \n", DecisionOnce},
	}

	req := &PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
		Description:  "HTTP GET",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			var writer bytes.Buffer
			p := NewTerminalPrompterWithIO(reader, &writer)

			decision, err := p.Prompt(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, decision)

			// Verify output contains expected elements
			output := writer.String()
			assert.Contains(t, output, "Permission Request")
			assert.Contains(t, output, "network")
			assert.Contains(t, output, "api.github.com")
		})
	}
}

func TestTerminalPrompterRejectsInvalidThenAccepts(t *testing.T) {
	// First line is invalid, second is valid
	input := "x\no\n"
	reader := strings.NewReader(input)
	var writer bytes.Buffer
	p := NewTerminalPrompterWithIO(reader, &writer)

	req := &PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceMCP,
		ResourceID:   "bash",
	}

	decision, err := p.Prompt(req)
	require.NoError(t, err)
	assert.Equal(t, DecisionOnce, decision)

	output := writer.String()
	assert.Contains(t, output, "Invalid input")
}

func TestTerminalPrompterNoInput(t *testing.T) {
	reader := strings.NewReader("")
	var writer bytes.Buffer
	p := NewTerminalPrompterWithIO(reader, &writer)

	req := &PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "example.com",
	}

	decision, err := p.Prompt(req)
	assert.Error(t, err)
	assert.Equal(t, DecisionDeny, decision)
}

func TestAutoDenyPrompter(t *testing.T) {
	p := &AutoDenyPrompter{}

	req := &PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "example.com",
	}

	decision, err := p.Prompt(req)
	require.NoError(t, err)
	assert.Equal(t, DecisionDeny, decision)
}
