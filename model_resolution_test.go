package main

import (
	"context"
	"testing"

	"github.com/codewandler/llm"
)

// MockProvider implements llm.Provider for testing
// resolveModel only needs ModelsProvider; CreateStream is unused.
type MockProvider struct {
	models llm.Models
}

func (m *MockProvider) Name() string       { return "mock" }
func (m *MockProvider) Models() llm.Models { return m.models }
func (m *MockProvider) CreateStream(ctx context.Context, req llm.Buildable) (llm.Stream, error) {
	return nil, nil
}

func TestResolveModel(t *testing.T) {
	mockModels := llm.Models{
		{
			ID:       "codex/gpt-5.4",
			Name:     "GPT-5.4",
			Provider: "codex",
			Aliases:  []string{"codex", "default"},
		},
		{
			ID:       "codex/gpt-5.4-mini",
			Name:     "GPT-5.4 Mini",
			Provider: "codex",
			Aliases:  []string{"mini"},
		},
		{
			ID:       "claude-sonnet-4-20250514",
			Name:     "Claude Sonnet 4 (2025-05-14)",
			Provider: "anthropic",
			Aliases:  []string{"sonnet"},
		},
	}

	provider := &MockProvider{models: mockModels}

	tests := []struct {
		name          string
		modelStr      string
		expectedID    string
		expectError   bool
		errorContains string
	}{
		{name: "exact ID match", modelStr: "codex/gpt-5.4", expectedID: "codex/gpt-5.4"},
		{name: "alias match - codex", modelStr: "codex", expectedID: "codex/gpt-5.4"},
		{name: "alias match - default", modelStr: "default", expectedID: "codex/gpt-5.4"},
		{name: "alias match - mini", modelStr: "mini", expectedID: "codex/gpt-5.4-mini"},
		{name: "alias match - sonnet", modelStr: "sonnet", expectedID: "claude-sonnet-4-20250514"},
		{name: "unknown model", modelStr: "unknown-model", expectError: true, errorContains: "unknown model"},
		{name: "empty string", modelStr: "", expectError: true, errorContains: "cannot be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveModel(provider, tt.modelStr)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				if tt.errorContains != "" && err != nil && !contains(err.Error(), tt.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expectedID {
					t.Errorf("expected %q, got %q", tt.expectedID, result)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
