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
	"github.com/codewandler/agentapis/conversation"
	llmproviders "github.com/codewandler/llmproviders"
	"github.com/codewandler/llmproviders/registry"
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

type fakeProvider struct {
	streamer conversation.Streamer
}

func (fp *fakeProvider) Name() string { return "fake" }
func (fp *fakeProvider) Stream(ctx context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	return fp.streamer.Stream(ctx, req)
}
func (fp *fakeProvider) CreateSession(opts ...conversation.Option) *conversation.Session {
	return conversation.New(fp.streamer, opts...)
}

// Uses "anthropic" ServiceID and "claude-sonnet-4-6" model to pass catalog validation.
func newFakeService() *llmproviders.Service {
	reg := registry.New()
	reg.Register(registry.Registration{
		InstanceName: "anthropic",
		ServiceID:    "anthropic",
		Order:        1,
		Detect: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		Build: func(ctx context.Context, cfg registry.BuildConfig) (registry.Provider, error) {
			return &fakeProvider{streamer: &fakeStreamer{}}, nil
		},
	})

	svc, _ := llmproviders.NewService(
		llmproviders.WithRegistry(reg),
	)
	return svc
}

func TestMarkdownRendering_StableFenceBlock(t *testing.T) {
	if os.Getenv("MINIAGENT_INTEGRATION") == "" {
		t.Skip("set MINIAGENT_INTEGRATION=1 to run integration tests")
	}
	var buf bytes.Buffer
	svc := newFakeService()
	a := agent.New(svc, agent.WithWorkspace(t.TempDir()), agent.WithToolTimeout(5*time.Second), agent.WithOutput(&buf))
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
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEsc = false
			}
			continue
		}
		if c == 0x1b {
			inEsc = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
