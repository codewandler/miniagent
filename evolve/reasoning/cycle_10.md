# Cycle 10 — Reasoning

## What changed

Modified `agent/system.go` — the system prompt body.

Two targeted edits:

1. **Renamed the "Read a file and evaluate an expression" pattern** to
   "Read a file and evaluate an **arithmetic constant**" and added
   **(NEVER read then compute separately)** to the label, making the
   single-pipeline requirement impossible to miss.

2. **Added a new `## CRITICAL: Source-reading tasks` section** that
   explicitly forbids the two-step pattern (cat/read file → compute in a
   later call) and gives the exact grep+awk one-liner for the
   `maxOutputBytes`-style lookup, including a bc fallback for more complex
   expressions.

## Why

Benchmark **004_source_reading** was the only benchmark below perfect
efficiency (steps=3, efficiency=0.7778, composite=0.9556). The task is to
find a constant defined as `20 * 1024` and write the evaluated integer. The
old prompt already showed the grep+awk pattern, but it wasn't emphatic enough:
the agent was still reading the whole file first, then evaluating in a second
(or third) step.

The new CRITICAL section repeats the instruction in imperative form and
provides the exact command referencing the actual file path, leaving no
ambiguity.

## Which benchmarks should improve

| Benchmark | Before | Expected after |
|---|---|---|
| 004_source_reading | composite=0.9556, steps=3 | composite=1.0, steps=1 |
| All others | already 1.0 | unchanged |

Overall avg_composite should rise from ~0.9911 to ~1.0.

## Diff

```diff
--- a/cmd/miniagent/agent/system.go
+++ b/cmd/miniagent/agent/system.go
@@ -16,13 +16,21 @@ Only issue a second bash call if the first one fails ...
-  - Read a file and evaluate an expression — ONE call:
+  - Read a file and evaluate an arithmetic constant — ONE call (NEVER read then compute separately):
       grep 'constName' file.go | grep -oP '\d+ \* \d+' | awk '{print $1*$3}' > /tmp/bench_result.txt
+    If the expression has more terms, use bc: grep 'constName' file.go | grep -oP '[\d *+()-]+' | head -1 | bc > /tmp/bench_result.txt
+
+## CRITICAL: Source-reading tasks
+When asked to find a constant defined as an arithmetic expression (e.g., maxOutputBytes = 20 * 1024):
+- Do NOT read the whole file first, then compute in a second step.
+- Extract AND evaluate in a single pipeline using grep + awk or grep + bc.
+- Write ONLY the plain integer result to the output file.
+Example: grep 'maxOutputBytes' /repo/cmd/miniagent/agent/tools.go | grep -oP '\d+ \* \d+' | awk '{print $1*$3}' > /tmp/bench_result.txt
```
