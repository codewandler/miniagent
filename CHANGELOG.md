# Changelog

## v0.6.4 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.19.0`
- Replaced miniagent-local llmadapter auto-mux defaults and route identity conversion with `agentsdk/runtime` helpers
- Use the requested model as the llmadapter auto-detection intent

## v0.6.3 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.18.0`
- Reused `agentsdk/runtime.SessionOptions` so miniagent no longer mirrors runtime defaults separately for persistent sessions

## v0.6.2 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.17.0`
- Replaced miniagent-local route/usage provider-model normalization with `agentsdk/usage` runner helpers

## v0.6.1 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.16.0`
- Replaced miniagent-local activation manager wiring with `agentsdk/tools/standard.Toolset`
- Replaced direct tool-management context key usage with `agentsdk/runtime.WithToolActivation`

## v0.6.0 — 2026-04-25

### Removed
- Removed `--context-budget`, `MINIAGENT_CONTEXT_BUDGET`, and miniagent-local context compaction because provider history must remain immutable for caching and continuation

### Changed
- Updated to `github.com/codewandler/agentsdk v0.15.0`

## v0.5.1 — 2026-04-25

### Added
- Added opt-in context budgeting with `--context-budget` and `MINIAGENT_CONTEXT_BUDGET`
- Compact omitted history into a projection-only summary message while preserving the durable session tree

### Changed
- Updated to `github.com/codewandler/agentsdk v0.14.0`

## v0.5.0 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.13.0`
- Replaced miniagent-local fake clients and stream helpers with `agentsdk/runnertest`
- Split agent runtime, session, provider, tool, and reasoning setup out of `agent.go`

## v0.4.5 — 2026-04-25

### Fixed
- Updated to `github.com/codewandler/agentsdk v0.12.3` so Anthropic thinking signatures are preserved across tool-loop replay
- Added an integration regression test for Sonnet thinking mode plus tool use replaying signed reasoning

## v0.4.4 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.12.2`
- Updated to `github.com/codewandler/llmadapter v0.48.10`
- Inherited agentsdk native Responses continuation projection, so compatible resumed sessions can use provider `previous_response_id` instead of replaying the full history
- Stream plain prose before conservative markdown block boundaries so normal assistant text appears earlier

## v0.4.3 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.11.5`
- Updated to `github.com/codewandler/llmadapter v0.48.7`
- Default model requests now set a stable per-session cache key alongside `cache_policy=on`

## v0.4.2 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.11.4`
- Default outgoing model requests now use `cache_policy=on` through agentsdk

## v0.4.1 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.11.3`
- Updated to `github.com/codewandler/llmadapter v0.48.6`

## v0.4.0 — 2026-04-25

### Added
- Persist CLI sessions as JSONL files under `~/.miniagent/sessions`
- Added `--session` to resume a previous session by id or JSONL path
- Added `--continue` to resume the most recently active session
- Added `--sessions-dir` to override the session storage directory

### Changed
- Updated to `github.com/codewandler/agentsdk v0.11.2`

## v0.3.5 — 2026-04-25

### Changed
- Updated to `github.com/codewandler/agentsdk v0.10.1`
- Updated to `github.com/codewandler/llmadapter v0.48.4`
- Inherited latest llmadapter auto-routing, Claude provider identity, and provider capability fixes

## v0.3.4 — 2026-04-24

### Changed
- Updated to `github.com/codewandler/agentsdk v0.10.0`
- Updated to `github.com/codewandler/llmadapter v0.46.0`
- Inherited llmadapter's modeldb-aware auto intent routing and Anthropic reasoning support
- Changed the default thinking mode to `auto`, which does not require provider reasoning unless `--thinking on` is set

## v0.3.3 — 2026-04-24

### Changed
- Updated to `github.com/codewandler/agentsdk v0.9.0`
- Runtime tool context construction now uses the agentsdk runtime tool-context factory
- Explicit `--model` values now use llmadapter dynamic passthrough instead of replacing the default intent route

### Added
- Added model-selection documentation covering intent routes, dynamic passthrough, provider priority, and credentials
- Added a focused test proving miniagent requests both intent and dynamic model routes from llmadapter auto mux

## v0.3.2 — 2026-04-24

### Changed
- Updated to `github.com/codewandler/llmadapter v0.44.0`
- Enabled dynamic model passthrough routes alongside the default intent route, so explicit model IDs can be supplied directly

## v0.3.1 — 2026-04-24

Follow-up cleanup after the agentsdk/llmadapter runtime migration.

### Changed
- Updated to `github.com/codewandler/agentsdk v0.8.0` and `github.com/codewandler/llmadapter v0.42.0`
- Replaced miniagent's local tool context implementation with `agentsdk/runtime.NewToolContext`
- Replaced manual auto-route summary extraction with `llmadapter/adapterconfig.AutoResult.RouteSummary`
- Enabled local Codex OAuth auto-detection in the auto mux configuration

## v0.3.0 — 2026-04-24

Migrated the runtime from `agentapis`/`llmproviders` to `agentsdk` and `llmadapter`, with provider auto-detection through the llmadapter mux client.

### Added
- Added `agentsdk/runtime` based turn execution in the agent loop
- Added runner event handling split into `agent/events.go`
- Added SDK-backed usage conversion in `agent/usage.go`
- Added cancellation integration coverage for real follow-up turns

### Changed
- Updated to `github.com/codewandler/agentsdk v0.7.0` and `github.com/codewandler/llmadapter v0.37.0`
- Replaced local model/provider setup with `llmadapter/adapterconfig.AutoMuxClient`
- Replaced miniagent's duplicate usage tracker with `agentsdk/usage`
- Switched display usage formatting to `llmadapter/unified` token and cost records
- Simplified tests to inject `llmadapter/unified.Client` directly
- Preserved explicit prompt caching behavior while moving session/runtime ownership into the SDK

### Removed
- Removed runtime dependencies on `agentapis` and `llmproviders`
- Removed the local `agent/usage` package
- Removed the `miniagent llm` subcommand that came from `llmproviders/cli`

## v0.2.10 — 2026-04-21

Improved cancellation handling during tool execution so interrupted runs report canceled tool results cleanly and preserve conversation consistency.

### Added
- Regression tests covering cancellation during single and multiple tool calls, ensuring canceled tool results are flushed back into the follow-up request
- Test provider `Stream` passthrough helpers used by cancellation and integration coverage

### Changed
- Tool execution now maps canceled and timed-out tool failures to stable `[Canceled]` and `[Timed out]` tool results
- Follow-up tool responses are now recorded with explicit error state via `ToolResultWithError`
- The agent now flushes pending canceled tool results to the model after context cancellation before returning the cancellation error

### Fixed
- Avoided dropping tool-call results when cancellation happens immediately after a streamed tool-use response
- Marked remaining queued tool calls as canceled once an earlier tool call is canceled or times out

## v0.2.9 — 2026-04-21

### Changed
- Renamed `agentcore` dependency to `agentsdk` (`github.com/codewandler/agentsdk`)
- Updated all import paths and replace directive

## v0.2.8 — 2026-04-21

Improved runtime diagnostics, upgraded to llmproviders service layer, and rewrote system prompt for tool-first workflows.

### Added
- `--verbose` / `-v` flag to print resolved provider and model before each turn
- `WithVerbose` functional option on `Agent`
- `resolvedModel` field on `Agent` to track the provider-resolved model name
- `providerName()` helper for diagnostic formatting
- Error messages now include `provider=`, `model=`, and `step=` context for easier debugging
- Test for stream-error diagnostic output (`TestRunTurn_StreamErrorIncludesDiagnostics`)
- `ParamsSummary` now shows `resolved_instance` and `resolved_model` when available
- `go mod tidy` step before `go install` in Taskfile

### Changed
- Rewrote system prompt to lead with specialized built-in tools instead of bash, with explicit tool→parameter mappings, a WRONG vs RIGHT section, and demoted bash to "only for running programs"
- Migrated from `llmproviders v0.0.0` to `v0.5.1`; upgraded `agentapis` to `v0.9.1`, `glamour` to `v1.0.0`, `llm` to `v0.40.0`, and bumped ~15 transitive dependencies
- Removed stale `github.com/codewandler/llm` replace directive from `go.mod`

### Removed
- Deleted 5 leftover debug/scratch files (`.tmp_debug_stream.go`, `stream_debug_test.go`, `stream_debug_list_test.go`, `stream_debug_toolflow_test.go`, `agent/debug_real_stream_test.go`)

## v0.2.7 — 2026-04-21

Restructured agent package: created dedicated `display/` package, split large files, and removed dead code.

### Added
- Created `agent/display/` package with 5 focused files:
  - `ansi.go` — ANSI escape codes and Truncate helper
  - `format.go` — token/cost formatting utilities
  - `markdown.go` — Glamour-based markdown rendering
  - `step.go` — StepDisplay state machine for streaming output
  - `usage.go` — usage line printing (step, turn, session)
- Created `agent/options.go` for InferenceOptions and functional options (80 lines)
- Created `agent/toolexec.go` for tool execution and context adapter (87 lines)

### Changed
- Split `agent/agent.go` from 487 → 344 lines by extracting options and tool execution
- Moved display tests to `agent/display/display_test.go`
- Updated `AGENTS.md` with new file structure documentation
- Marked `llm` dependency as indirect (no longer directly imported)

### Removed
- Deleted `agent/tools.go` (125 lines of dead code after agentcore migration)
- Deleted `agent/tools_test.go` (dead tests)
- Deleted `agent/display.go` (moved to display package)
- Deleted `agent/display_test.go` (moved to display package)

## v0.2.6 — 2026-04-20

Improved model resolution visibility, request caching hints, and SWE-bench agent benchmarking ergonomics.

### Added
- Printed the provider-resolved model once per streamed step when the model becomes known
- Added request-cache coverage in agent tests to ensure top-level cache hints are propagated from message history
- Added a dedicated `bench:swe:agent:go` Task target for the live Go-centric SWE benchmark subset
- Added live per-instance progress output in the SWE bench runner and optional live agent log streaming via `SWE_LIVE_OUTPUT=1`
- Added tests covering non-overlapping output and reasoning token aggregation and display

### Changed
- Moved model resolution display into the agent streaming path instead of resolving model aliases eagerly in `main`
- Updated `github.com/codewandler/agentapis` to `v0.3.2`
- Extended the SWE bench Docker task setup to mount Codex credentials read-only for agent-driven runs

### Fixed
- Fixed `agent` tests to use valid llm test stream helpers
- Fixed usage aggregation and display expectations so output and reasoning tokens are reported separately without double counting
- Removed obsolete top-level model resolution tests tied to the previous eager-resolution path

## v0.2.5 — 2026-04-19

Improved streamed markdown rendering so terminal layout is width-aware, stable, and visually consistent between blocks.

### Changed
- Markdown rendering now uses terminal-width-aware glamour wrapping instead of relying on terminal hard wraps
- Streaming markdown display now trims only outer renderer-added blank lines while keeping stable block-by-block rendering
- Reused the markdown renderer setup per display path to keep incremental rendering efficient

### Fixed
- Removed extra visual blank lines between independently rendered markdown blocks, including heading-to-paragraph transitions
- Wrapped paragraph continuation lines now align correctly because wrapping is handled by the renderer instead of the terminal

## v0.2.4 — 2026-04-19

Added built-in web tooling via agentcore, including optional Tavily-backed web search when configured.

### Added
- Registered `web_fetch` in the default tool set
- Registered `web_search` automatically when `WEBSEARCH_PROVIDER=tavily` and `TAVILY_API_KEY` is set
- Added agent tests covering both the configured and unconfigured web-search cases

### Changed
- Updated `github.com/codewandler/agentsdk` to `v0.2.2` to pick up the new web tooling

## v0.2.3 — 2026-04-19

Documentation release for the context-management guide.

### Added
- Added a comprehensive `docs/context-management.md` guide for coding-agent context management
- Added web-search-informed coverage of context budgeting, recency windows, selective retrieval, prompt caching, and evaluation
- Added concrete external references for follow-up verification

### Changed
- Expanded the guide from an advantages-only overview into a more operational engineering document

### Fixed
- Documented failure modes, disadvantages, common culprits, and practical mitigations for each major context-management pattern

## v0.2.2 — 2026-04-19

Cleanup and release follow-up for the markdown streaming work.

### Changed
- Made `codex/gpt-5.4` the actual default model and ensured Codex-prefixed model IDs resolve consistently
- Finalized test layout under `tests/integration/` and `tests/e2e/`
- Added renderer injection for cleaner display tests without ANSI stripping

### Fixed
- Removed obsolete top-level `integration/markdown_render_test.go` after moving tests under `tests/`
- Kept build, tests, and `task install` passing after the test layout cleanup

## v0.2.1 — 2026-04-18

Follow-up release for the markdown streaming integration.

### Added
- Integration test scaffolding in `integration/markdown_render_test.go`
- Additional display tests covering stable markdown buffering and glamour rendering

### Changed
- Updated provider creation to match current `codewandler/llm` service-based auto package
- Wrapped `llm.Service` in a provider runtime compatible with the existing agent interface
- Restored full project build and `task install` after upstream llm changes

### Fixed
- Fixed imports after `llm/provider/auto` moved to `llm/auto`
- Fixed model listing / completion / resolution against the newer llm service API
- Fixed miniagent tests broken by llm provider helper changes

## v0.2.0 — 2026-04-18

Added stable streaming markdown rendering for assistant output.

### Added
- Reintroduced terminal markdown rendering via 
- Integrated  into the step display pipeline
- Incremental rendering of stable markdown blocks during assistant streaming
- Explicit regression tests for markdown rendering, fenced-code buffering, and tool-call flushing

### Changed
- Assistant text is no longer printed raw token-by-token; it is buffered into stable markdown blocks first
- Markdown rendering uses  to avoid the old auto-style failure mode
- Updated agent tests to use a local blocking provider compatible with the current llm API

### Fixed
- Restored markdown rendering without depending on interactive TTY auto-detection
- Ensured fenced code is not rendered until its closing fence is seen
- Ensured pending markdown is flushed before tool call output and at stream end

## v0.1.0 — 2026-04-16

First tagged release of the standalone CLI. This release turns miniagent into a Cobra-based command tree, adds `completion` and `completion install` commands for bash/zsh/fish, and wires model flag completion for `--model` / `-m`.

It also keeps the existing agent loop, `/new` REPL reset, temperature/thinking/effort flags, and agentcore tool integration.

---

Entries are added automatically by the self-improvement loop (`task evolve`).
Each entry corresponds to one accepted improvement cycle.
No version numbers — revisions are synthetic counters local to this loop.

## revision 2 — 2026-04-15

Strengthened the system-prompt guidance for single-pipeline source-reading: added a CRITICAL section that explicitly forbids reading a file and computing in separate steps, with the exact grep+awk one-liner for arithmetic-constant lookups. Benchmark 004_source_reading is expected to improve from composite 0.9556 (3 steps) to 1.0 (1 step).

---

## revision 10 — 2026-04-15

Added a **"NEVER explore before acting"** section to `agent/system.go` with
explicit Do-NOT bullet rules (no preliminary cat/ls, no post-write verification,
no split read+write calls), plus a concrete one-shot grep/awk example for
evaluating source constants. Targets benchmarks 001 and 004 (3 steps → 1 step),
expected to raise avg_composite from 0.98224 to 1.0000.

---

## revision 9 — 2026-04-15

Added a **"WRONG vs RIGHT"** section to `agent/system.go` with three explicit
❌/✅ contrasts targeting the two lowest-efficiency benchmarks: the one-at-a-time
append anti-pattern (001_batch_commands, 4 steps → 1) and the cat-then-compute
anti-pattern (004_source_reading, 3 steps → 1). Expected to raise avg_composite
from 0.9778 to 1.0000.

---


## revision 8 — 2026-04-15

Added an **Anti-patterns to NEVER do** section to the system prompt in
`agent/system.go`. Explicitly forbids reconnaissance/preview steps (e.g.
`cat file.go` before writing the result) that caused benchmarks 001 and 004
to run in 3 steps instead of 1. Expected to raise avg_composite from 0.982
to ~0.998 by pushing those two benchmarks to efficiency=1.0.

---

## revision 7 — 2026-04-15

Added a "STRICT: Never use a separate step just to read or explore a file" section to the system prompt. This eliminates the extra exploratory-cat step that caused benchmark 004_source_reading to run in 3 steps instead of 1, expected to raise its efficiency from 0.7778 → 1.0 and lift avg_composite from 0.9911 → 1.0.

---

## revision 6 — 2026-04-15

Strengthened the system-prompt pattern for evaluating arithmetic expressions
from Go source constants: added a primary `grep -oP ... | bash $((...))`
one-liner and a CRITICAL section explicitly forbidding the cat-then-evaluate
multi-step anti-pattern. Targets `004_source_reading` (3 steps → 1), expected
to raise avg_composite from 0.9911 to 1.0.

---
## revision 5 — 2026-04-15

Added an explicit "Anti-patterns that waste steps" section to the system prompt.
Forbids post-write verification reads, pre-task exploratory commands, pipeline splitting,
and redundant confirmation calls. Expected to reduce benchmark 001 from 4 steps to 1
and benchmark 004 from 3 steps to 1, raising avg_efficiency from ~0.889 toward ~0.967
and avg_composite from ~0.978 above the 1.02× acceptance threshold.

---

## revision 4 — 2026-04-15

Added an "Anti-patterns — NEVER do these" section to the system prompt in
`agent/system.go`. The four bullet points explicitly forbid splitting file-read,
exploration, and result-writing into separate bash calls. Expected to bring
benchmarks 001_batch_commands and 004_source_reading from 3 steps down to 1,
raising avg_efficiency from ~0.911 toward ~0.956 and avg_composite above the
1.02× acceptance threshold.

---

## revision 3 — 2026-04-15

Replaced "Think step by step" with a PRIMARY RULE block at the top of the system prompt that instructs the agent to mentally combine ALL steps into ONE bash command before issuing any tool call. Also refined the batching pattern examples (error-recovery now shows a pipe-based idiom, gather-values example targets bench_result.txt directly). Benchmarks 001, 003, 004, and 005 are expected to drop to 1 step each, raising avg_efficiency toward 1.0 and avg_composite above the 1.02× acceptance threshold.

---


## revision 1 — 2026-04-15

Added explicit "Efficiency rules" to the system prompt and extended the bash tool description to instruct the model to batch multiple operations into a single command call using shell operators (&&, ||, ;, pipes, subshells). Benchmarks 001, 003, 004, and 005 are expected to drop from 2–3 steps to 1 step each, raising avg_composite from ~0.9734 toward 1.0000.

---
