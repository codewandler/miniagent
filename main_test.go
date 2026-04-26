package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/stretchr/testify/require"
)

func TestMiniagentResourcesLoadPrimaryAgent(t *testing.T) {
	resolved, err := agentdir.ResolveFS(miniagentResources, ".agents")
	require.NoError(t, err)
	name, err := resolved.ResolveDefaultAgent("")
	require.NoError(t, err)
	require.Equal(t, "coder", name)

	application, err := app.New(app.WithBundle(resolved.Bundle), app.WithDefaultAgent(name))
	require.NoError(t, err)
	spec, ok := application.AgentSpec(name)
	require.True(t, ok)
	require.Contains(t, spec.System, "helpful terminal assistant")
}

func TestMiniagentResourcesDoNotComeFromWorkingDirectory(t *testing.T) {
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(oldWD)) })

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".agents", "agents", "wrong.md"), []byte("---\nname: wrong\n---\nwrong"), 0o644))
	require.NoError(t, os.Chdir(dir))

	resolved, err := agentdir.ResolveFS(miniagentResources, ".agents")
	require.NoError(t, err)
	name, err := resolved.ResolveDefaultAgent("")
	require.NoError(t, err)
	require.Equal(t, "coder", name)
}
