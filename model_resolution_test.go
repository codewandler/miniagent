package main

import (
	"context"
	"testing"

	"github.com/codewandler/llm"
)

// MockProvider implements llm.Provider for testing
type MockProvider struct {
	models llm.Models
}

func (m *MockProvider) Name() string {
	return "mock"
}

func (m *MockProvider) Models() llm.Models {
	return m.models
}

func (m *MockProvider) CreateStream(ctx context.Context, req llm.Buildable) (llm.Stream, error) {
	return nil, nil
}

func TestResolveModel(t *testing.T) {
	mockModels := llm.Models{
		{
			ID:       "claude-opus-4-5-20250514",
			Name:     "Claude Opus 4.5 (2025-05-14)",
			Provider: "anthropic",
			Aliases:  []string{"opus", "default"},
		},
		{
			ID:       "claude-sonnet-4-20250514",
			Name:     "Claude Sonnet 4 (2025-05-14)",
			Provider: "anthropic",
			Aliases:  []string{"sonnet"},
		},
		{
			ID:       "claude-haiku-4-5-20251001",
			Name:     "Claude Haiku 4.5 (2025-10-01)",
			Provider: "anthropic",
			Aliases:  []string{"haiku"},
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
		{
			name:       "exact ID match",
			modelStr:   "claude-opus-4-5-20250514",
			expectedID: "claude-opus-4-5-20250514",
		},
		{
			name:       "alias match - opus",
			modelStr:   "opus",
			expectedID: "claude-opus-4-5-20250514",
		},
		{
			name:       "alias match - default",
			modelStr:   "default",
			expectedID: "claude-opus-4-5-20250514",
		},
		{
			name:       "alias match - sonnet",
			modelStr:   "sonnet",
			expectedID: "claude-sonnet-4-20250514",
		},
		{
			name:       "alias match - haiku",
			modelStr:   "haiku",
			expectedID: "claude-haiku-4-5-20251001",
		},
		{
			name:          "unknown model",
			modelStr:      "unknown-model",
			expectError:   true,
			errorContains: "unknown model",
		},
		{
			name:          "empty string",
			modelStr:      "",
			expectError:   true,
			errorContains: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveModel(provider, tt.modelStr)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				if tt.errorContains != "" && err != nil {
					if err.Error() == "" || !contains(err.Error(), tt.errorContains) {
						t.Errorf("error message %q does not contain %q", err.Error(), tt.errorContains)
					}
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
