package integration

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/agentapis/api/unified"
	"github.com/codewandler/agentapis/client"
	"github.com/codewandler/miniagent/agent"
	"github.com/stretchr/testify/require"
)

type fakeStreamer struct{ n int }

func (f *fakeStreamer) Stream(_ context.Context, _ unified.Request) (<-chan client.StreamResult, error) {
	ch := make(chan client.StreamResult, 2)
	go func() {
		defer close(ch)
		if f.n == 0 {
			ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventContentDelta, ContentDelta: &unified.ContentDelta{ContentBase: unified.ContentBase{Kind: unified.ContentKindText, Data: "done"}}}}
			ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventCompleted, Completed: &unified.Completed{StopReason: unified.StopReasonEndTurn}}}
		}
		f.n++
	}()
	return ch, nil
}

func TestMarkdownRendering_StableFenceBlock(t *testing.T) {
	if os.Getenv("MINIAGENT_INTEGRATION") == "" {
		t.Skip("set MINIAGENT_INTEGRATION=1 to run integration tests")
	}
	var buf bytes.Buffer
	a := agent.New(&fakeStreamer{}, agent.WithWorkspace(t.TempDir()), agent.WithToolTimeout(5*time.Second), agent.WithOutput(&buf))
	err := a.RunTurn(context.Background(), 1, "show code")
	require.NoError(t, err)
	out := buf.String()
	stripped := stripANSI(out)
	require.Contains(t, stripped, "done")
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEsc {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') { inEsc = false }
			continue
		}
		if c == 0x1b { inEsc = true; continue }
		b.WriteByte(c)
	}
	return b.String()
}
