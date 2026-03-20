package daemon

import (
	"testing"

	"github.com/dpopsuev/misbah/config"
	"github.com/stretchr/testify/assert"
)

func TestNewNetworkIsolator(t *testing.T) {
	cfg := config.NetworkSection{
		Bridge: "misbah0",
		Subnet: "10.88.0.0/24",
		MTU:    1500,
	}
	ni := NewNetworkIsolator(cfg, testLogger())
	assert.NotNil(t, ni)
	assert.Equal(t, "misbah0", ni.cfg.Bridge)
}

func TestNetworkIsolator_NsName_Empty(t *testing.T) {
	cfg := config.NetworkSection{Subnet: "10.88.0.0/24"}
	ni := NewNetworkIsolator(cfg, testLogger())
	assert.Empty(t, ni.NsName("nonexistent"))
}

func TestNetworkIsolator_TeardownNonexistent(t *testing.T) {
	cfg := config.NetworkSection{Subnet: "10.88.0.0/24"}
	ni := NewNetworkIsolator(cfg, testLogger())
	err := ni.Teardown("nonexistent")
	assert.Error(t, err)
}

func TestNetworkIsolator_DuplicateSetup(t *testing.T) {
	cfg := config.NetworkSection{Subnet: "invalid"}
	ni := NewNetworkIsolator(cfg, testLogger())

	// Force a duplicate check by pre-populating
	ni.netns["test"] = &isolatedNet{nsName: "test"}
	_, err := ni.Setup("test", "127.0.0.1:8118")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestNetworkIsolator_InvalidSubnet(t *testing.T) {
	cfg := config.NetworkSection{Subnet: "not-a-cidr"}
	ni := NewNetworkIsolator(cfg, testLogger())

	_, err := ni.Setup("test", "127.0.0.1:8118")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid subnet")
}
