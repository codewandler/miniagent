package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSystemPrompt(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		custom    string
		wantAll   []string
		wantNone  []string
	}{
		{
			name:      "default includes workspace and bash mention",
			workspace: "/home/user/project",
			custom:    "",
			wantAll:   []string{"/home/user/project", "bash", "## Workspace"},
		},
		{
			name:      "custom replaces body but keeps workspace",
			workspace: "/tmp/work",
			custom:    "You are a pirate assistant.",
			wantAll:   []string{"You are a pirate assistant.", "/tmp/work", "## Workspace"},
			wantNone:  []string{"helpful terminal assistant"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSystemPrompt(tt.workspace, tt.custom)
			for _, s := range tt.wantAll {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.wantNone {
				assert.NotContains(t, got, s)
			}
		})
	}
}
