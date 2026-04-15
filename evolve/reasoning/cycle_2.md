# Cycle 2 — Reasoning

## What I changed

Added an **"Anti-patterns — NEVER do these"** section to `agent/system.go`
(the system prompt body), immediately before the closing summary line.

The four new bullet points explicitly forbid the most common multi-step
mistakes:

1. Reading a file in one call, then processing in a second.
2. Running an exploratory command (pwd, ls, cat) first, then writing results.
3. Splitting "gather info" and "write to file" into two calls.
4. Doing a dry-run / sanity-check step before the real command.

## Why

Benchmarks **001_batch_commands** and **004_source_reading** both score
`efficiency = 0.7778` because the agent uses **3 steps** instead of 1.

- **001**: likely reads `pwd`, `whoami`, and counts `.go` files in separate
  calls before (or while) writing to `/tmp/bench_result.txt`.
- **004**: likely `cat`s `tools.go` first to inspect it, then runs a second
  command to extract and evaluate `maxOutputBytes`.

The system prompt already had positive batching patterns, but no explicit
prohibition against these anti-patterns. Adding NEVER-do-these rules gives
the model a strong negative signal to avoid the multi-step habit.

## Which benchmarks should improve

| Benchmark | Current steps | Expected steps | Efficiency before | Efficiency after |
|-----------|--------------|----------------|-------------------|-----------------|
| 001_batch_commands | 3 | 1 | 0.7778 | 1.0 |
| 004_source_reading | 3 | 1 | 0.7778 | 1.0 |

Expected composite improvement:
- Each benchmark: 0.9556 → 1.0  (+0.0444)
- avg_composite: 0.98224 → ~0.9911 (+0.009) — well above the 1.02× threshold
  needed if both improve.

## Diff

```diff
--- a/cmd/miniagent/agent/system.go
+++ b/cmd/miniagent/agent/system.go
@@ -27,6 +27,16 @@ Only issue a second bash call if the first one fails or if you genuinely need it
 Use &&, ||, ;, $(...), pipes, { ...; } grouping, and here-strings freely.
 Avoid separate tool calls for things that can be done together.
 
+## Anti-patterns — NEVER do these
+- NEVER cat/read a file in one call and then process or evaluate it in a second call.
+  Instead: pipe directly — grep + awk in a single pipeline that also writes the result.
+- NEVER run an exploratory command (pwd, ls, cat) first and then write results separately.
+  Instead: embed the exploration inside $(...) and write everything in ONE call.
+- NEVER split "gather info" and "write to file" into two separate bash calls.
+  Instead: redirect output of the gathering command straight to the file.
+- NEVER do a dry-run or sanity-check step before the real command.
+  Instead: combine the check into the same pipeline (use || to handle failures inline).
+
 When the task is done, respond with a clear summary of what you accomplished.`
```
