package agent

import (
	"context"
	"time"

	"github.com/codewandler/agentsdk/activation"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
)

const (
	canceledToolResult = "[Canceled]"
	timedOutToolResult = "[Timed out]"
)

func (a *Agent) newToolCtx(ctx context.Context) *agentcoreToolContext {
	toolCtx := &agentcoreToolContext{
		ctx:        ctx,
		workspace:  a.workspace,
		activation: a.activation,
		extra:      make(map[string]any),
		sessionID:  a.sessionID,
	}
	toolCtx.extra[toolmgmt.KeyActivationState] = a.activation
	return toolCtx
}

// agentcoreToolContext adapts context.Context for agentsdk tools.
type agentcoreToolContext struct {
	ctx        context.Context
	workspace  string
	activation activation.State
	extra      map[string]any
	sessionID  string
}

func (c *agentcoreToolContext) WorkDir() string       { return c.workspace }
func (c *agentcoreToolContext) Extra() map[string]any { return c.extra }
func (c *agentcoreToolContext) Deadline() (time.Time, bool) {
	if c.ctx == nil {
		return time.Time{}, false
	}
	return c.ctx.Deadline()
}
func (c *agentcoreToolContext) Done() <-chan struct{} {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Done()
}
func (c *agentcoreToolContext) Err() error {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}
func (c *agentcoreToolContext) Value(key any) any {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Value(key)
}
func (c *agentcoreToolContext) AgentID() string   { return "" }
func (c *agentcoreToolContext) SessionID() string { return c.sessionID }
