package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveSessionPathContinueUsesNewestSession(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "20260425T100000Z-old.jsonl")
	newPath := filepath.Join(dir, "20260425T110000Z-new.jsonl")
	require.NoError(t, os.WriteFile(oldPath, []byte("{}\n"), 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte("{}\n"), 0o600))
	oldTime := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(oldPath, oldTime, oldTime))
	require.NoError(t, os.Chtimes(newPath, newTime, newTime))

	path, err := resolveSessionPath(dir, "", true)
	require.NoError(t, err)
	require.Equal(t, newPath, path)
}

func TestResolveSessionPathFindsSessionID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260425T110000Z-abc123.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o600))

	got, err := resolveSessionPath(dir, "abc123", false)
	require.NoError(t, err)
	require.Equal(t, path, got)
}

func TestResolveContextBudgetPrefersFlag(t *testing.T) {
	t.Setenv("MINIAGENT_CONTEXT_BUDGET", "100")
	got, err := resolveContextBudget(42)
	require.NoError(t, err)
	require.Equal(t, 42, got)
}

func TestResolveContextBudgetFromEnv(t *testing.T) {
	t.Setenv("MINIAGENT_CONTEXT_BUDGET", "100")
	got, err := resolveContextBudget(0)
	require.NoError(t, err)
	require.Equal(t, 100, got)
}

func TestResolveContextBudgetRejectsInvalidEnv(t *testing.T) {
	t.Setenv("MINIAGENT_CONTEXT_BUDGET", "nope")
	_, err := resolveContextBudget(0)
	require.Error(t, err)
}
