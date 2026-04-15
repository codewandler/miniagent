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
