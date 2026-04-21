package agent

import (
	"context"

	"github.com/codewandler/agentapis/api/unified"
	"github.com/codewandler/agentapis/client"
	"github.com/codewandler/agentapis/conversation"
	llmproviders "github.com/codewandler/llmproviders"
	"github.com/codewandler/llmproviders/registry"
)

// testFakeStreamer is a minimal streamer for testing
type testFakeStreamer struct{}

func (t testFakeStreamer) Stream(ctx context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	ch := make(chan client.StreamResult, 2)
	go func() {
		defer close(ch)
		ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventContentDelta, ContentDelta: &unified.ContentDelta{ContentBase: unified.ContentBase{Kind: unified.ContentKindText, Data: "test response"}}}}
		ch <- client.StreamResult{Event: unified.StreamEvent{Type: unified.StreamEventCompleted, Completed: &unified.Completed{StopReason: unified.StopReasonEndTurn}}}
	}()
	return ch, nil
}

func (t testFakeStreamer) Name() string { return "test" }

// testFakeProvider wraps testFakeStreamer
type testFakeProvider struct {
	streamer conversation.Streamer
}

func (fp *testFakeProvider) Name() string { return "test" }
func (fp *testFakeProvider) Stream(ctx context.Context, req unified.Request) (<-chan client.StreamResult, error) {
	return fp.streamer.Stream(ctx, req)
}
func (fp *testFakeProvider) CreateSession(opts ...conversation.Option) *conversation.Session {
	return conversation.New(fp.streamer, opts...)
}

// TestServiceID is the service ID used for test fake providers.
// Uses "anthropic" since it's in the modeldb catalog.
const TestServiceID = "anthropic"

// TestModelID is the model ID used for test fake providers.
// Uses a real model ID that exists in the modeldb catalog.
const TestModelID = "claude-sonnet-4-6"

// newFakeService creates a minimal Service for testing.
// Uses "anthropic" ServiceID and "claude-sonnet-4-6" model to pass catalog validation.
func newFakeService() *llmproviders.Service {
	reg := registry.New()
	reg.Register(registry.Registration{
		InstanceName: TestServiceID,
		ServiceID:    TestServiceID,
		Order:        1,
		Detect: func(ctx context.Context) (bool, error) {
			return true, nil
		},
		Build: func(ctx context.Context, cfg registry.BuildConfig) (registry.Provider, error) {
			return &testFakeProvider{streamer: testFakeStreamer{}}, nil
		},
	})

	svc, _ := llmproviders.NewService(
		llmproviders.WithRegistry(reg),
	)
	return svc
}
