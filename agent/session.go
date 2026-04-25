package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/conversation/jsonlstore"
	acoreTool "github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

func (a *Agent) initSession(ctx context.Context) error {
	if a.resumeSession == "" && a.sessionStoreDir == "" {
		return nil
	}
	opts := a.conversationOptions(false)
	if a.resumeSession != "" {
		store := jsonlstore.Open(a.resumeSession)
		session, err := conversation.Resume(ctx, store, "", opts...)
		if err != nil {
			return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
		}
		a.session = session
		a.sessionID = string(session.SessionID())
		a.sessionStorePath = a.resumeSession
		return nil
	}
	return a.startPersistentSession(time.Now())
}

func (a *Agent) startPersistentSession(now time.Time) error {
	if a.sessionStoreDir == "" {
		a.session = nil
		a.sessionStorePath = ""
		return nil
	}
	path := filepath.Join(a.sessionStoreDir, fmt.Sprintf("%s-%s.jsonl", now.UTC().Format("20060102T150405Z"), a.sessionID))
	store := jsonlstore.Open(path)
	opts := append(a.conversationOptions(true),
		conversation.WithStore(store),
		conversation.WithConversationID(conversation.ConversationID("conv_"+a.sessionID)),
	)
	a.session = conversation.New(opts...)
	a.sessionStorePath = path
	return nil
}

func (a *Agent) conversationOptions(includeSessionID bool) []conversation.Option {
	opts := []conversation.Option{
		conversation.WithModel(a.inference.Model),
		conversation.WithMaxOutputTokens(a.inference.MaxTokens),
		conversation.WithTemperature(a.inference.Temperature),
		conversation.WithSystem(BuildSystemPrompt(a.workspace, a.systemOverride)),
		conversation.WithTools(acoreTool.UnifiedToolsFrom(a.toolset.ActiveTools())),
		conversation.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
		conversation.WithCachePolicy(unified.CachePolicyOn),
		conversation.WithCacheKey(a.cacheKey()),
	}
	if includeSessionID {
		opts = append([]conversation.Option{conversation.WithSessionID(conversation.SessionID(a.sessionID))}, opts...)
	}
	if reasoning, ok := a.reasoningConfig(); ok {
		opts = append(opts, conversation.WithReasoning(reasoning))
	}
	return opts
}

func (a *Agent) cacheKey() string {
	if a.sessionID == "" {
		return ""
	}
	return "miniagent:" + a.sessionID
}
