package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractAptPackages(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{"install single", []string{"install", "nginx"}, []string{"nginx"}},
		{"install multiple", []string{"install", "nginx", "curl", "wget"}, []string{"nginx", "curl", "wget"}},
		{"install with flags", []string{"install", "-y", "nginx"}, []string{"nginx"}},
		{"install with version", []string{"install", "nginx=1.18.0-1"}, []string{"nginx"}},
		{"apt-get install", []string{"install", "-y", "--no-install-recommends", "build-essential"}, []string{"build-essential"}},
		{"update (no packages)", []string{"update"}, nil},
		{"no install", []string{"list", "--installed"}, nil},
		{"empty", []string{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPackages(PackageManagerApt, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPipPackages(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{"install single", []string{"install", "numpy"}, []string{"numpy"}},
		{"install multiple", []string{"install", "numpy", "pandas", "scipy"}, []string{"numpy", "pandas", "scipy"}},
		{"install with version", []string{"install", "numpy==1.24.0"}, []string{"numpy"}},
		{"install with >=", []string{"install", "numpy>=2.0"}, []string{"numpy"}},
		{"install with ~=", []string{"install", "numpy~=1.24"}, []string{"numpy"}},
		{"install with -r", []string{"install", "-r", "requirements.txt"}, nil},
		{"install with --upgrade", []string{"install", "--upgrade", "pip"}, []string{"pip"}},
		{"install with -e", []string{"install", "-e", "./mypackage"}, nil},
		{"install with target", []string{"install", "-t", "/tmp/libs", "requests"}, []string{"requests"}},
		{"list (no packages)", []string{"list"}, nil},
		{"freeze", []string{"freeze"}, nil},
		{"empty", []string{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPackages(PackageManagerPip, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractNpmPackages(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{"install single", []string{"install", "express"}, []string{"express"}},
		{"install multiple", []string{"install", "express", "lodash"}, []string{"express", "lodash"}},
		{"i shorthand", []string{"i", "express"}, []string{"express"}},
		{"add", []string{"add", "express"}, []string{"express"}},
		{"install with version", []string{"install", "express@4.18.0"}, []string{"express"}},
		{"install with scoped", []string{"install", "@types/node@18.0.0"}, []string{"@types/node"}},
		{"install with --save-dev", []string{"install", "--save-dev", "jest"}, []string{"jest"}},
		{"install no packages (bare)", []string{"install"}, nil},
		{"run (no packages)", []string{"run", "test"}, nil},
		{"test", []string{"test"}, nil},
		{"empty", []string{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPackages(PackageManagerNpm, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPackages_UnknownManager(t *testing.T) {
	result := ExtractPackages("cargo", []string{"install", "ripgrep"})
	assert.Nil(t, result)
}

func TestStripPipVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"numpy", "numpy"},
		{"numpy==1.24.0", "numpy"},
		{"numpy>=2.0", "numpy"},
		{"numpy<=3.0", "numpy"},
		{"numpy~=1.24", "numpy"},
		{"numpy!=1.0", "numpy"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripPipVersion(tt.input))
		})
	}
}

func TestPackageWrapper_AllowedPackage(t *testing.T) {
	checker := newMockChecker().withWhitelist(ResourcePackage, "true", DecisionAlways)
	pw := NewPackageWrapper(checker, "test-container", testLogger())

	decision, err := pw.checkPermission(context.Background(), "true")
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, decision)
}

func TestPackageWrapper_DeniedPackage(t *testing.T) {
	checker := newMockChecker()
	pw := NewPackageWrapper(checker, "test-container", testLogger())

	decision, err := pw.checkPermission(context.Background(), "malware-pkg")
	require.NoError(t, err)
	assert.Equal(t, DecisionDeny, decision)
}

func TestPackageWrapper_Caching(t *testing.T) {
	cp := newCountingPrompter(DecisionAlways)
	checker := newMockChecker().withPrompter(cp.prompt)
	pw := NewPackageWrapper(checker, "test-container", testLogger())

	_, err := pw.checkPermission(context.Background(), "numpy")
	require.NoError(t, err)

	_, err = pw.checkPermission(context.Background(), "numpy")
	require.NoError(t, err)

	assert.Equal(t, int32(1), cp.count.Load())
}

func TestPackageWrapper_OnceNotCached(t *testing.T) {
	cp := newCountingPrompter(DecisionOnce)
	checker := newMockChecker().withPrompter(cp.prompt)
	pw := NewPackageWrapper(checker, "test-container", testLogger())

	_, err := pw.checkPermission(context.Background(), "numpy")
	require.NoError(t, err)

	_, err = pw.checkPermission(context.Background(), "numpy")
	require.NoError(t, err)

	assert.Equal(t, int32(2), cp.count.Load())
}

func TestWrapperScript(t *testing.T) {
	script := WrapperScript(PackageManagerPip, "/usr/bin/pip3", "/run/misbah/permission.sock", "agent-main")

	assert.Contains(t, script, "#!/bin/sh")
	assert.Contains(t, script, "MISBAH_DAEMON_SOCKET=\"/run/misbah/permission.sock\"")
	assert.Contains(t, script, "MISBAH_CONTAINER=\"agent-main\"")
	assert.Contains(t, script, "misbah-pkg-check pip /usr/bin/pip3")
}
