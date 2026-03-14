package mount

import (
	"runtime"
	"testing"

	"github.com/jabal/jabal/metrics"
	"github.com/jabal/jabal/model"
	"github.com/stretchr/testify/assert"
)

func TestNewNamespaceManager(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)
	assert.NotNil(t, nm)
	assert.NotNil(t, nm.logger)
}

func TestNamespaceManagerCheckNamespaceSupport(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	err := nm.CheckNamespaceSupport()

	if runtime.GOOS != "linux" {
		// Should fail on non-Linux
		assert.Error(t, err)
	} else {
		// On Linux, may pass or fail depending on system configuration
		// We just verify it doesn't panic
		_ = err
	}
}

func TestNamespaceManagerBuildMountScript(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	sources := []model.Source{
		{Path: "/tmp/source1", Mount: "source1"},
		{Path: "/tmp/source2", Mount: "source2"},
	}

	script := nm.buildMountScript("/tmp/jabal/test", sources)

	assert.Contains(t, script, "mkdir -p \"/tmp/jabal/test\"")
	assert.Contains(t, script, "mkdir -p \"/tmp/jabal/test/source1\"")
	assert.Contains(t, script, "mount --bind \"/tmp/source1\" \"/tmp/jabal/test/source1\"")
	assert.Contains(t, script, "mkdir -p \"/tmp/jabal/test/source2\"")
	assert.Contains(t, script, "mount --bind \"/tmp/source2\" \"/tmp/jabal/test/source2\"")
}

func TestNamespaceManagerCreateNamespace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Namespace tests require Linux")
	}

	// Note: Full integration tests for CreateNamespace are in test/integration/
	// because they require actual namespace creation which may fail in some environments
	t.Skip("Integration tests for CreateNamespace are in test/integration/")
}
