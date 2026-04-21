package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/codewandler/agentapis/api/unified"
	"github.com/codewandler/agentcore/interfaces"
	acoreTool "github.com/codewandler/agentcore/tool"
	"github.com/codewandler/agentcore/tools/toolmgmt"
)

func (a *Agent) executeTool(ctx context.Context, tc unified.ToolCall) (string, bool) {
	for _, t := range a.activation.ActiveTools() {
		if t.Name() != tc.Name {
			continue
		}
		input, _ := json.Marshal(tc.Args)
		toolCtx := &agentcoreToolContext{ctx: ctx, workspace: a.workspace, activation: a.activation, extra: make(map[string]any), sessionID: a.sessionID}
		toolCtx.extra[toolmgmt.KeyActivationState] = a.activation
		result, err := t.Execute(toolCtx, input)
		if err != nil {
			return err.Error(), true
		}
		return result.String(), result.IsError()
	}
	return fmt.Sprintf("tool not found: %s", tc.Name), true
}

func (a *Agent) activeToolSpecs() []unified.Tool {
	active := a.activation.ActiveTools()
	out := make([]unified.Tool, 0, len(active))
	for _, t := range active {
		out = append(out, convertUnifiedToolDefinition(t))
	}
	return out
}

func convertUnifiedToolDefinition(t acoreTool.Tool) unified.Tool {
	schema := t.Schema()
	raw, _ := json.Marshal(schema)
	var params map[string]any
	_ = json.Unmarshal(raw, &params)
	delete(params, "$schema")
	delete(params, "$id")
	return unified.Tool{Name: t.Name(), Description: t.Description(), Parameters: params}
}

// agentcoreToolContext adapts context.Context for agentcore tools.
type agentcoreToolContext struct {
	ctx        context.Context
	workspace  string
	activation interfaces.ActivationState
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
