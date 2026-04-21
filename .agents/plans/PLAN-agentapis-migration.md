# PLAN: migrate `miniagent` to `agentapis` + `llmproviders`

## Purpose

Migrate `miniagent` away from:
- `github.com/codewandler/llm`
- `github.com/codewandler/llm/msg`
- `github.com/codewandler/llm/tool`
- `github.com/codewandler/llm/usage`

and onto:
- `github.com/codewandler/agentapis/conversation`
- `github.com/codewandler/agentapis/api/unified`
- `github.com/codewandler/agentcore`
- `github.com/codewandler/llmproviders/providers/codex`
- `github.com/codewandler/llmproviders/pricing`

This migration is the first real application-level proof that the new architecture works.

---

## Why this is the next milestone

At this point:
- `llmproviders` exists
- `codex` exists as a real first provider
- codex has model metadata and pricing
- codex has smoke, tool-use, and continuation probes
- `agentapis/conversation` has become more agent-facing, including `Session.Events(...)`

So the next architectural proof is not more provider work.

It is this:

> can `miniagent` run a real multi-step tool loop on top of `agentapis/conversation` and `llmproviders/providers/codex`, without depending on legacy `llm` abstractions?

That is the milestone this plan is for.

---

## Updated design target from current `agentapis`

The current in-flight `agentapis` changes matter here.

Most important refinement:
- `conversation.Session` now exposes `Events(...)`
- `Events(...)` emits a compact, agent-facing event stream
- this is a better target for `miniagent` than consuming raw unified stream events directly

### Current agent-facing conversation event surface
Relevant event types include:
- `TextDeltaEvent`
- `ReasoningDeltaEvent`
- `ToolCallEvent`
- `TransportUsageEvent`
- `TurnUsageEvent`
- `CompletedEvent`
- `ErrorEvent`

That means `miniagent` should target:
- `conversation.Session`
- `conversation.Events(...)`

instead of:
- local `msg.Messages`
- direct low-level provider loop logic

---

## Migration goal

A successful migration means `miniagent` can:
- create a `conversation.Session`
- submit a user turn
- receive compact conversation events
- execute tool calls via `agentcore`
- feed tool results back through `conversation.Request`
- continue until completion
- accumulate usage from transport usage events
- compute cost using `llmproviders/pricing`
- run against `llmproviders/providers/codex`

with no dependency on `codewandler/llm` in the main agent loop.

---

## Scope

### In scope
- new provider/session seam
- replacement of local message history with `conversation.Session`
- event-loop migration to `conversation.Events(...)`
- codex as first backend
- usage/cost migration to unified transport usage + `llmproviders/pricing`
- tool conversion away from `llm/tool`
- enough refactor to run one real tool loop end-to-end

### Out of scope for first pass
- support all providers immediately
- preserve identical internal structure
- solve every display refinement
- remove every legacy import in one step if temporary shims are needed during refactor
- perfect cost attribution across every mode before the loop works

---

## Current state in `miniagent`

Today `miniagent` still centers around:
- `llm.Provider`
- `msg.Messages`
- `llm/tool.Definition`
- `usage.Tracker`

Conversation state is currently managed manually.
Tool loops are manually stitched into the local message history.
This is exactly the old architecture we want to move away from.

---

## Target architecture in `miniagent`

## Core dependencies
Target imports should center on:
- `github.com/codewandler/agentapis/conversation`
- `github.com/codewandler/agentapis/api/unified`
- `github.com/codewandler/agentcore/tool`
- `github.com/codewandler/llmproviders/providers/codex`
- `github.com/codewandler/llmproviders/pricing`

## New core agent shape
Conceptually:

```go
type Agent struct {
    streamer    conversation.Streamer
    session     *conversation.Session
    allTools    []tool.Tool
    activation  *ActivationManager
    maxSteps    int
    out         io.Writer
    workspace   string
    toolTimeout time.Duration
    sessionID   string

    usage SessionUsage
}
```

Notably absent in the target shape:
- no `llm.Provider`
- no `msg.Messages`
- no `llm/tool.Definition`
- no `llm/usage.Tracker`

---

## New agent seams

### 1. Provider seam
Replace `llm.Provider` with either:
- `conversation.Streamer`
- or a tiny app-local abstraction wrapping `conversation.Streamer`

Recommended choice:
- use `conversation.Streamer` directly where practical

This keeps `miniagent` aligned with `agentapis` instead of inventing a new app-specific provider model.

### 2. Conversation seam
Replace local history with:
- `conversation.Session`

This means:
- system prompt becomes session default state
- user turns become `conversation.Request`
- tool-result followups become `conversation.Request`
- replay/native continuation is delegated to `agentapis`

### 3. Event seam
Consume:
- `conversation.Events(...)`

instead of manually reducing raw provider stream events.

### 4. Tool seam
Replace `llm/tool.Definition` usage with:
- direct conversion from `agentcore` tools into `unified.Tool`

### 5. Usage seam
Replace `llm/usage` with:
- `conversation.TransportUsageEvent`
- optional `conversation.TurnUsageEvent`
- cost calculation via `llmproviders/pricing`

---

## Event-driven loop target

The new main loop should consume compact conversation events.

### Relevant events
#### `TextDeltaEvent`
Use for assistant output streaming.

#### `ReasoningDeltaEvent`
Use for optional reasoning display.

#### `ToolCallEvent`
Collect tool calls to execute after the stream completes.

#### `TransportUsageEvent`
Capture exact provider-reported usage for cost and usage reporting.

#### `CompletedEvent`
Marks logical turn completion.

#### `ErrorEvent`
Fail the current turn and preserve/cancel as appropriate.

### Key benefit
This lets `miniagent` stop reimplementing stream-to-agent event reduction.

---

## Recommended internal file split

Current `agent.go` is overloaded.
Recommended split:

```text
miniagent/agent/
  agent.go
  session.go
  runner.go
  provider.go
  tools.go
  usage.go
  display.go
```

### `agent.go`
Keep the public `Agent` type and constructor wiring.

### `session.go`
Own:
- `conversation.Session` construction
- reset behavior
- model/system/tool defaults

### `runner.go`
Own:
- one-turn execution loop
- event consumption
- tool-call collection
- tool-result continuation

### `provider.go`
Own:
- codex provider construction
- future backend selection/configuration

### `tools.go`
Own:
- active tool collection
- conversion from `agentcore` tool metadata to `unified.Tool`

### `usage.go`
Own:
- session/turn usage accumulation
- pricing lookup and cost calculation

### `display.go`
Own:
- rendering text deltas
- rendering reasoning deltas
- rendering tool call banners
- rendering usage/cost summaries

---

## Detailed migration plan

## Phase 1 — introduce seam without full behavior change
Goal:
- decouple `miniagent` from direct `llm.Provider` assumptions

Checklist:
- [ ] define new provider/session seam in `miniagent`
- [ ] stop tying constructor directly to `llm.Provider`
- [ ] make room for `conversation.Session` inside `Agent`
- [ ] isolate current message-history-specific logic so it can be replaced

Deliverable:
- `miniagent` compiles with a cleaner seam for backend replacement

---

## Phase 2 — replace conversation history with `conversation.Session`
Goal:
- stop managing `msg.Messages` directly

Checklist:
- [ ] create session in `New(...)`
- [ ] pass system prompt via `conversation.WithSystem(...)`
- [ ] pass tools via `conversation.WithTools(...)`
- [ ] pass model via `conversation.WithModel(...)`
- [ ] pass capabilities based on codex provider
- [ ] replace reset logic with `session.Reset()`
- [ ] remove local history append logic from `RunTurn`

Deliverable:
- conversation state is owned by `agentapis`

---

## Phase 3 — migrate to `conversation.Events(...)`
Goal:
- consume compact event stream instead of legacy stream/provider logic

Checklist:
- [ ] use `session.Events(ctx, req)` for a turn
- [ ] stream `TextDeltaEvent` into current display layer
- [ ] stream `ReasoningDeltaEvent` into reasoning display path
- [ ] collect `ToolCallEvent`s during the turn
- [ ] collect `TransportUsageEvent`s during the turn
- [ ] stop on `CompletedEvent`
- [ ] fail on `ErrorEvent`

Deliverable:
- `miniagent` agent loop runs on agent-facing conversation events

---

## Phase 4 — tool execution continuation
Goal:
- preserve the multi-step agent loop using conversation-native followups

Checklist:
- [ ] execute collected tool calls via `agentcore`
- [ ] map tool outputs into `conversation.NewRequest().ToolResult(...)`
- [ ] resubmit followup turns via session
- [ ] keep iterating until no tool calls remain
- [ ] preserve existing max-step semantics
- [ ] re-check rollback/error behavior against conversation semantics

Deliverable:
- one real multi-step tool loop works end-to-end

---

## Phase 5 — migrate tools away from `llm/tool`
Goal:
- use `agentcore` + `unified.Tool` only

Checklist:
- [ ] define conversion from `agentcore` tool metadata/schema to `unified.Tool`
- [ ] replace `toolDefs []tool.Definition` with `[]unified.Tool` or derived-on-demand conversion
- [ ] remove `github.com/codewandler/llm/tool` from the main agent loop

Deliverable:
- tool transport shape is `unified.Tool`
- tool execution shape remains `agentcore`

---

## Phase 6 — usage and cost migration
Goal:
- stop using legacy `llm/usage`

Checklist:
- [ ] collect `TransportUsageEvent` usage values
- [ ] aggregate usage per turn
- [ ] aggregate usage per session
- [ ] map current model/provider to `llmproviders/pricing`
- [ ] compute estimated cost using codex pricing
- [ ] replace usage display helpers accordingly
- [ ] remove `github.com/codewandler/llm/usage` from the new path

Deliverable:
- session and turn usage/cost summaries work on the new stack

---

## Phase 7 — codex backend proof
Goal:
- make codex the first real backend for migrated `miniagent`

Checklist:
- [ ] construct `llmproviders/providers/codex` in `miniagent`
- [ ] set codex-derived capabilities on the session
- [ ] run one non-tool turn against codex
- [ ] run one tool-use turn against codex
- [ ] verify continuation behavior through conversation session

Deliverable:
- `miniagent` works against codex through the new stack

---

## Minimum viable migration target

The first acceptable end-state is not a total rewrite.
It is this smaller proof:

- [ ] one user turn works
- [ ] one tool call is emitted
- [ ] one tool result continuation works
- [ ] final answer is rendered
- [ ] transport usage is captured
- [ ] estimated cost is shown
- [ ] backend is `llmproviders/providers/codex`
- [ ] no dependency on `llm/msg` in the active turn loop

If this works, the architecture is proven enough to continue cleanup.

---

## Interaction with current `agentapis` changes

These current `agentapis` updates directly affect the migration design:

### `conversation.Events(...)`
This should become the main event surface for `miniagent`.

### `conversation/events.go`
This introduces a compact agent-facing event API, which means `miniagent` should not consume richer unified events unless it actually needs them.

### `conversation/session.go` and tests
Recent tests show `agentapis` is actively improving:
- tool-call persistence
- replay/native continuation behavior
- `PreviousResponseID` handling
- multi-step tool loops

This is good news for the migration.
It means `miniagent` should lean more heavily on `agentapis` instead of keeping old local logic alive.

### Design implication
The migration target is now clearer than before:
- `Session` owns conversation state
- `Events(...)` is the primary agent loop surface
- `miniagent` focuses on orchestration, tools, display, and policy

---

## Risks and cautions

### 1. Mixed migration period
During refactor, `miniagent` may temporarily contain both:
- legacy loop behavior
- new conversation-based loop behavior

Recommendation:
- isolate the new path clearly
- avoid half-reusing old message-history logic inside the new loop

### 2. Usage display differences
`conversation.TransportUsageEvent` is exact provider transport usage, not the same shape as legacy `usage.Record`.

Recommendation:
- prefer correctness over preserving exact old display formatting at first

### 3. Tool definition schema conversion
If `agentcore` and `unified.Tool` drift, conversion could become noisy.

Recommendation:
- keep the conversion local and explicit first
- extract later only if it stabilizes

### 4. Legacy tests may overfit old internals
Some current tests likely assert on:
- local `messages`
- rollback shape
- tracker internals

Recommendation:
- update tests toward externally visible behavior where possible

---

## Success criteria

The migration is successful when:
- `miniagent` uses `conversation.Session`
- `miniagent` consumes `conversation.Events(...)`
- codex works as a backend through `llmproviders`
- one real multi-step tool loop works end-to-end
- transport usage is captured and rendered
- cost can be estimated using `llmproviders/pricing`
- the main loop no longer depends on `llm/msg`
- the new stack is simpler than the old one, not just equivalent

---

## Strong recommendation

Do not start by trying to perfectly preserve every internal legacy behavior.

Instead:
1. prove one real conversation/tool loop on the new stack
2. get codex working as the first backend
3. then clean up remaining legacy structure and tests

That will keep the migration focused on architectural validation rather than accidental detail compatibility.
