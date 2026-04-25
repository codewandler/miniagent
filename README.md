# miniagent

A terminal AI assistant that understands plain-English tasks, runs bash
commands to carry them out, and keeps going until the job is done.

## What it can do

- Read, explain, and navigate codebases
- Write, refactor, and fix code across multiple files
- Run builds, tests, linters, and pipelines
- Create, move, and reorganise files and directories
- Debug failing commands and recover from errors
- Anything you would do in a terminal — just described in plain English

## Install

```sh
task install
```

Generate and install shell completions:

```sh
miniagent completion install
miniagent completion install zsh
```

You need one LLM API key — set any of:

```sh
export ANTHROPIC_API_KEY=sk-ant-…
# or: OPENAI_API_KEY / OPENROUTER_API_KEY
```

## Use

```sh
# give it a task and it runs until done
miniagent "add structured logging to the auth package"

# interactive session — type tasks one by one
miniagent

# point at a specific project folder
miniagent --workspace /path/to/project "write tests for main.go"
```

## Sessions

Interactive and one-shot runs can persist sessions as JSONL files:

```sh
miniagent "start investigating the failing tests"
miniagent --continue "keep going from the last session"
miniagent --session 20260425T120000Z-abc123.jsonl "continue this session"
miniagent --sessions-dir /tmp/miniagent-sessions --continue "continue there"
```

The default session store is `~/.miniagent/sessions`. `--continue` resumes the
most recently active session in that directory, preserving the session id and
cache key.

miniagent uses `cache_policy=on` by default with a stable per-session cache key.
Conversation history is not trimmed, compacted, or summarized automatically;
provider-visible history is kept stable so caching, native continuation, and
tool-call replay stay correct.

## Models

miniagent builds its model client with `llmadapter` auto-detection. It looks for
environment keys and local OAuth credentials for OpenAI, Codex, Anthropic,
Claude, OpenRouter, and other registered providers, then creates a mux client.

The default model flag value is an intent route. llmadapter resolves it to the
best detected provider/model for this machine:

```sh
miniagent "say pong"
```

You can also pass a model alias or explicit provider-native model ID. The
requested model is passed into llmadapter auto-detection as the intent:

```sh
miniagent -m haiku "say pong"
miniagent -m gpt-5.4 "say pong"
miniagent -m claude-sonnet-4-6 "say pong"
```

Provider priority and exact native-model support come from `llmadapter`'s auto
mux selection. Useful credentials include:

```sh
export OPENAI_API_KEY=...
export ANTHROPIC_API_KEY=...
export OPENROUTER_API_KEY=...
```

Local Claude/Codex OAuth credentials are also detected when available.

## Self-improvement

miniagent can improve itself over time. Run the evolution loop and it will
read its own source code, propose a change, test whether the change actually
makes it better, and — if it does — commit the improvement and keep going.

```sh
task evolve                   # run indefinitely  (Ctrl-C to stop)
task evolve -- --cycles 5    # run exactly 5 improvement cycles
```

Requires Docker. Each accepted improvement is committed to the current
branch with a plain description of what changed and why.

See [AGENTS.md](AGENTS.md) for how the loop works and how to extend it.
