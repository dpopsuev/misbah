package daemon

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Prompter asks the user for a permission decision.
type Prompter interface {
	Prompt(req *PermissionRequest) (Decision, error)
}

// TerminalPrompter displays to a writer and reads from a reader.
type TerminalPrompter struct {
	reader io.Reader
	writer io.Writer
}

// NewTerminalPrompter creates a prompter using stdin/stderr.
func NewTerminalPrompter() *TerminalPrompter {
	return &TerminalPrompter{
		reader: os.Stdin,
		writer: os.Stderr,
	}
}

// NewTerminalPrompterWithIO creates a prompter with custom I/O (for tests).
func NewTerminalPrompterWithIO(reader io.Reader, writer io.Writer) *TerminalPrompter {
	return &TerminalPrompter{
		reader: reader,
		writer: writer,
	}
}

// Prompt displays the permission request and reads the user's choice.
func (t *TerminalPrompter) Prompt(req *PermissionRequest) (Decision, error) {
	scanner := bufio.NewScanner(t.reader)

	for {
		fmt.Fprintf(t.writer, "\nPermission Request [%s]\n", req.ResourceType)
		fmt.Fprintf(t.writer, "  Container: %s\n", req.Container)
		fmt.Fprintf(t.writer, "  Resource:  %s\n", req.ResourceID)
		if req.Description != "" {
			fmt.Fprintf(t.writer, "  Description: %s\n", req.Description)
		}
		fmt.Fprintf(t.writer, "\n  [O]nce  [A]lways  [D]eny: ")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return DecisionDeny, fmt.Errorf("failed to read input: %w", err)
			}
			return DecisionDeny, fmt.Errorf("no input received")
		}

		input := strings.TrimSpace(strings.ToLower(scanner.Text()))

		switch input {
		case "o", "once":
			return DecisionOnce, nil
		case "a", "always":
			return DecisionAlways, nil
		case "d", "deny":
			return DecisionDeny, nil
		default:
			fmt.Fprintf(t.writer, "  Invalid input %q. Please enter O, A, or D.\n", input)
		}
	}
}

// AutoDenyPrompter always returns Deny (for tests and non-interactive mode).
type AutoDenyPrompter struct{}

// Prompt always returns DecisionDeny.
func (a *AutoDenyPrompter) Prompt(req *PermissionRequest) (Decision, error) {
	return DecisionDeny, nil
}
