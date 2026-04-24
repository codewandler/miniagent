package agent

import (
	"context"

	"github.com/codewandler/llmadapter/unified"
)

const TestServiceID = "test"
const TestModelID = "test-model"

type testFakeClient struct{}

func (t testFakeClient) Request(ctx context.Context, req unified.Request) (<-chan unified.Event, error) {
	ch := make(chan unified.Event, 3)
	go func() {
		defer close(ch)
		ch <- unified.TextDeltaEvent{Text: "test response"}
		ch <- unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "msg_test"}
	}()
	return ch, nil
}

func newFakeClient() unified.Client {
	return testFakeClient{}
}

type recordingClient struct {
	requests []unified.Request
	streams  [][]unified.Event
}

func (r *recordingClient) Request(_ context.Context, req unified.Request) (<-chan unified.Event, error) {
	r.requests = append(r.requests, req)
	idx := len(r.requests) - 1
	var items []unified.Event
	if idx < len(r.streams) {
		items = r.streams[idx]
	}
	ch := make(chan unified.Event, len(items))
	for _, item := range items {
		ch <- item
	}
	close(ch)
	return ch, nil
}
