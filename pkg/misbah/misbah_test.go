package misbah

import (
	"testing"
)

func TestNew_CreatesClient(t *testing.T) {
	m := New("/tmp/nonexistent.sock")
	defer m.Close()

	if m.client == nil {
		t.Fatal("client is nil")
	}
	if m.socketPath != "/tmp/nonexistent.sock" {
		t.Fatalf("socketPath = %q", m.socketPath)
	}
}

func TestAvailable_ReturnsFalseWhenNoSocket(t *testing.T) {
	m := New("/tmp/nonexistent-misbah-test.sock")
	defer m.Close()

	if m.Available() {
		t.Fatal("should return false when socket doesn't exist")
	}
}

func TestNewSpec_SensibleDefaults(t *testing.T) {
	spec := NewSpec("test-container", []string{"bash", "-c", "echo hello"})

	if spec.Version != "1.0" {
		t.Fatalf("version = %q", spec.Version)
	}
	if spec.Metadata.Name != "test-container" {
		t.Fatalf("name = %q", spec.Metadata.Name)
	}
	if len(spec.Process.Command) != 3 || spec.Process.Command[0] != "bash" {
		t.Fatalf("command = %v", spec.Process.Command)
	}
	if spec.Process.Cwd != "/workspace" {
		t.Fatalf("cwd = %q", spec.Process.Cwd)
	}
	if !spec.Namespaces.User || !spec.Namespaces.Mount {
		t.Fatal("user and mount namespaces should be enabled")
	}
	if spec.Runtime != "namespace" {
		t.Fatalf("runtime = %q", spec.Runtime)
	}

	// Validate the spec itself.
	if err := spec.Validate(); err != nil {
		t.Fatalf("spec validation: %v", err)
	}
}

func TestNewSpecWithTier_IncludesTierConfig(t *testing.T) {
	spec := NewSpecWithTier("tier-test", []string{"echo"}, "com",
		[]string{"/home/user/project"}, []string{"/workspace/src"})

	if spec.TierConfig == nil {
		t.Fatal("tier config is nil")
	}
	if spec.TierConfig.Tier != "com" {
		t.Fatalf("tier = %q", spec.TierConfig.Tier)
	}
	if len(spec.TierConfig.WritablePaths) != 1 || spec.TierConfig.WritablePaths[0] != "/workspace/src" {
		t.Fatalf("writable paths = %v", spec.TierConfig.WritablePaths)
	}
	if len(spec.Mounts) != 1 || spec.Mounts[0].Source != "/home/user/project" {
		t.Fatalf("mounts = %v", spec.Mounts)
	}
}

func TestExecResult_ZeroValue(t *testing.T) {
	var r ExecResult
	if r.ExitCode != 0 || r.Stdout != "" || r.Stderr != "" {
		t.Fatal("zero value should be clean")
	}
}
