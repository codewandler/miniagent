package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/llm/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateBytes(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		max       int
		truncated bool
	}{
		{"under limit", "hello", 100, false},
		{"at limit", "hello", 5, false},
		{"over limit", strings.Repeat("x", 200), 100, true},
		{"empty", "", 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateBytes([]byte(tt.input), tt.max)
			if tt.truncated {
				assert.Contains(t, result, "[...truncated")
				assert.Contains(t, result, "200 bytes")
			} else {
				assert.Equal(t, tt.input, result)
				assert.NotContains(t, result, "truncated")
			}
		})
	}
}

func TestTruncateBytes_LargeOutput(t *testing.T) {
	// Simulate a command producing output larger than maxOutputBytes (20 KB)
	big := strings.Repeat("x", 30*1024) // 30 KB
	result := truncateBytes([]byte(big), maxOutputBytes)
	assert.Contains(t, result, "[...truncated")
	assert.Contains(t, result, "30720 bytes")
	// The visible content is maxOutputBytes + the marker line
	assert.Less(t, len(result), 25*1024, "truncated result should be much smaller than input")
}

func TestNewBashHandler(t *testing.T) {
	workspace := t.TempDir()
	timeout := 5 * time.Second

	tests := []struct {
		name        string
		command     string
		timeout     time.Duration
		wantContain string
	}{
		{"echo", "echo hello", timeout, "hello"},
		{"non-zero exit", "exit 42", timeout, "exit 42"},
		{"timeout", "sleep 10", 100 * time.Millisecond, "timeout"},
		{"workspace is cwd", "pwd", timeout, workspace},
		{"stderr captured", "echo err >&2", timeout, "err"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewBashHandler(workspace, tt.timeout)
			call := tool.NewToolCall("test-id", "bash", map[string]any{"command": tt.command})
			out, err := handler.Handle(context.Background(), call)
			require.NoError(t, err, "handler must never return a Go error")
			assert.Contains(t, fmt.Sprint(out), tt.wantContain)
		})
	}
}

func TestBashDefinition(t *testing.T) {
	def := BashDefinition()
	assert.Equal(t, "bash", def.Name)
	assert.NotEmpty(t, def.Description)
}
