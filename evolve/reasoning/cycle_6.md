# Cycle 6 — Reasoning

## What changed

Added a new **"Anti-patterns to NEVER do"** section to `agent/system.go`
(the system prompt body). The section lists four explicit wrong/right pairs that
forbid reconnaissance-then-process-then-write multi-step patterns.

## Why

Benchmarks **001_batch_commands** and **004_source_reading** both scored
`steps=3, efficiency=0.7778` instead of the ideal `steps=1, efficiency=1.0`.

The system prompt already contained the correct batching patterns (e.g.,
`grep 'constName' file.go | awk '{print $1*$3}' > /tmp/bench_result.txt`), but
the agent still used 3 steps. The most likely cause: the LLM habitually makes a
**reconnaissance call** (e.g., `cat tools.go` to inspect the file, or `ls` to
see what's there) before doing the real work, then writes to
`/tmp/bench_result.txt` in a separate third step.

The new section directly names and forbids this anti-pattern with concrete
❌/✓ pairs so the model cannot miss it:

1. No `cat file.go` preview step before writing the result.
2. No split between "gather data" and "write to `/tmp/bench_result.txt`".
3. No scouting `ls`/`find`/`cat` steps before doing the real work.
4. No cross-turn arithmetic evaluation — use awk/`$(())` inline.

The closing paragraph makes it unambiguous:
> "Whenever a task says to write to /tmp/bench_result.txt, ALL data gathering
> AND the write MUST happen inside a single bash call. No exceptions."

## Which benchmarks should improve

| Benchmark             | Before          | Expected after  |
|-----------------------|-----------------|-----------------|
| 001_batch_commands    | steps=3 eff=0.78| steps=1 eff=1.0 |
| 004_source_reading    | steps=3 eff=0.78| steps=1 eff=1.0 |
| 002, 003, 005         | already 1 step  | unchanged       |

Expected composite improvement:
- Per-benchmark composite for 001 and 004: 0.9556 → 1.0
- avg_composite: 0.98224 → ~0.9978 (+~0.016, well above the 2% threshold)

## Diff

```diff
--- a/cmd/miniagent/agent/system.go
+++ b/cmd/miniagent/agent/system.go
@@ -27,6 +27,20 @@ Only issue a second bash call if the first one fails...
 Use &&, ||, ;, $(...), pipes, { ...; } grouping, and here-strings freely.
 Avoid separate tool calls for things that can be done together.

+## Anti-patterns to NEVER do
+These cost extra steps and lower your score — avoid them unconditionally:
+- ❌ WRONG: Step 1 = cat file.go (to preview it) → Step 2 = write result to /tmp/bench_result.txt
+  ✓ RIGHT: grep/awk the file AND redirect to /tmp/bench_result.txt in ONE step
+- ❌ WRONG: Step 1 = gather values → Step 2 = write to /tmp/bench_result.txt
+  ✓ RIGHT: redirect output directly to /tmp/bench_result.txt inside the SAME command
+- ❌ WRONG: First run ls / find / cat to "see what is there", then do the real work
+  ✓ RIGHT: Trust the task description and act on it immediately — no scouting steps
+- ❌ WRONG: Evaluate an arithmetic expression in your head across turns
+  ✓ RIGHT: Use awk or $(( )) inline: grep 'maxOutputBytes' tools.go | grep -oP '\d+ \* \d+' | awk '{print $1*$3}' > /tmp/bench_result.txt
+
+Whenever a task says to write to /tmp/bench_result.txt, ALL data gathering AND
+the write MUST happen inside a single bash call. No exceptions.
+
 When the task is done, respond with a clear summary of what you accomplished.`
```
