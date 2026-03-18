package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findSubcommand(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}

func TestRootCmdSubcommands(t *testing.T) {
	var names []string
	for _, cmd := range rootCmd.Commands() {
		names = append(names, cmd.Name())
	}

	assert.True(t, findSubcommand(names, "container"), "should have container subcommand")
	assert.True(t, findSubcommand(names, "daemon"), "should have daemon subcommand")
	assert.True(t, findSubcommand(names, "image"), "should have image subcommand")
	assert.True(t, findSubcommand(names, "version"), "should have version subcommand")
}

func TestContainerCmdSubcommands(t *testing.T) {
	var names []string
	for _, cmd := range containerCmd.Commands() {
		names = append(names, cmd.Name())
	}

	expected := []string{"create", "validate", "start", "stop", "list", "inspect", "destroy"}
	for _, exp := range expected {
		assert.True(t, findSubcommand(names, exp), "container should have %s subcommand", exp)
	}
}

func TestDaemonCmdSubcommands(t *testing.T) {
	var names []string
	for _, cmd := range daemonCmd.Commands() {
		names = append(names, cmd.Name())
	}

	assert.True(t, findSubcommand(names, "start"), "daemon should have start subcommand")
}

func TestImageCmdSubcommands(t *testing.T) {
	var names []string
	for _, cmd := range imageCmd.Commands() {
		names = append(names, cmd.Name())
	}

	expected := []string{"pull", "list", "inspect", "prune"}
	for _, exp := range expected {
		assert.True(t, findSubcommand(names, exp), "image should have %s subcommand", exp)
	}
}

func TestVersionCmd(t *testing.T) {
	rootCmd.SetArgs([]string{"version"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerCreateRequiresSpec(t *testing.T) {
	rootCmd.SetArgs([]string{"container", "create"})
	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestContainerStartRequiresSpec(t *testing.T) {
	rootCmd.SetArgs([]string{"container", "start"})
	err := rootCmd.Execute()
	assert.Error(t, err)
}
