# AGENTS.md — miniagent internals

This file is for developers and AI agents working on miniagent itself.
It describes how the codebase is structured, how the evolution loop operates,
and what the rules are for self-modification.

---

## Architecture

miniagent runs a tight loop:

```
user task → LLM → (optional) bash tool call → result back to LLM → repeat
```

The LLM gets one tool: **bash** — it runs arbitrary shell commands in the
configured workspace directory and receives combined stdout+stderr back.
The loop continues until the model stops calling the tool (task done) or
`--max-steps` is reached.

### Key source files

| File | Role |
|---|---|
| `main.go` | CLI entry point, flag definitions, provider setup |
| `agent/system.go` | **System prompt** — the highest-leverage file for self-improvement |
| `agent/tools.go` | Bash tool definition, executor, output truncation |
| `agent/agent.go` | Core LLM→tool loop, history management, streaming |
| `agent/display.go` | Terminal output formatting, ANSI colours |
| `agent/repl.go` | Interactive REPL mode |

---

## Self-improvement loop

Triggered by `task evolve`. Runs entirely inside a hardened Docker container
(no root, no Linux capabilities, read-only image filesystem, repo bind-mounted).

### One cycle, step by step

```
1. BASELINE    run benchmarks/ with the current stable binary → composite score
2. REASON      stable agent reads its source + scores, proposes ONE targeted change
3. IMPLEMENT   agent edits source files, then runs `go build` to verify compilation
               if build fails → auto-revert with git restore, skip to next cycle
4. BUILD       compile candidate binary (static, trimpath, stripped)
5. BENCHMARK   same benchmark suite against the candidate binary → composite score
6. JUDGE       candidate_composite > stable_composite × 1.02 ?
   KEEP  →  update CHANGELOG.md + docs → git commit → promote candidate to stable
   REVERT →  git restore source files → candidate discarded → next cycle
```

### Scoring formula

```
composite  = completed × 0.40 + correct × 0.40 + efficiency × 0.20
efficiency = clamp(1 − steps ÷ 10, 0, 1)
```

`completed` — did the agent exit 0?  
`correct` — does `/tmp/bench_result.txt` contain the `EXPECTED:` string?  
`steps` — number of bash tool calls made (fewer = more efficient)

### Stable binary lifecycle

On first run the current compiled binary is bootstrapped as the stable
baseline and stored at `evolve/bin/miniagent-stable`. After each accepted
cycle the candidate replaces it. The revision counter at `evolve/revision`
tracks how many improvements have been accepted since the loop started.

---

## Benchmarks

Each file in `benchmarks/*.md` is one benchmark. Format:

```markdown
# Benchmark: short name
EXPECTED: the string that must appear in /tmp/bench_result.txt

Plain-English task description for the agent.
Tell it to write the expected value to /tmp/bench_result.txt.
```

The `EXPECTED:` line is stripped before the task is passed to the agent —
the agent never sees it. If omitted, completing successfully (exit 0) is
enough to score `correct=1`.

To add a benchmark: drop a `.md` file into `benchmarks/` — it is picked up
automatically on the next `task evolve`.

---

## Rules for self-modification

These rules are enforced by the prompt given to the reasoning agent and must
be respected by any agent modifying this codebase:

**Allowed:**
- Edit `agent/system.go` — system prompt body
- Edit `agent/tools.go` — tool description, `maxOutputBytes`, timeout logic
- Edit `agent/display.go` — output formatting
- Edit `main.go` — CLI flag defaults
- Add new functions within existing files if they serve the allowed changes

**Not allowed:**
- Modify `agent/agent.go` or `agent/repl.go` — the core loop is off-limits
- Add new entries to `go.mod` / `go.sum` — no new external dependencies
- Modify benchmark files or files under `evolve/`
- Make more than one logical change per cycle

---

## Documentation and changelog conventions

Every accepted improvement must update:

1. **`CHANGELOG.md`** — add an entry at the top:
   ```
   ## revision N — YYYY-MM-DD

   What changed and why. Which benchmarks improved and by how much.

   ---
   ```
   Use the revision number from `evolve/revision`. No semver, no git tags —
   this project lives inside a larger monorepo.

2. **`README.md`** — update the *What it can do* list **only** if a
   user-visible capability was added, removed, or meaningfully changed.

3. **`AGENTS.md`** (this file) — update **only** if the architecture,
   evolution process, or agent rules changed.

---

## Runtime artefacts (gitignored)

```
.go-cache/        Go build cache (populated on first evolve run)
.miniagent-bin    Docker build artifact
evolve/bin/       stable + candidate binaries
evolve/scores/    per-benchmark JSON results
evolve/cycles.jsonl  numeric score history
```

`evolve/reasoning/` and `evolve/revision` **are** committed — they form
the record of what the agent tried and what revision it is on.
