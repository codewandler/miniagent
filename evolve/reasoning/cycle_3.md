# Cycle 3 — Reasoning

## What I changed

Added an **"Anti-patterns that waste steps — NEVER do these"** section to the system prompt in `agent/system.go`.

The four anti-patterns explicitly listed:
1. Reading a file back after writing it to verify the contents (trust the write succeeded)
2. Running an exploratory command (ls, cat, pwd) before the main task when the path is already given in the instructions
3. Splitting a single pipeline into multiple sequential bash calls
4. Writing a result and then issuing another call just to confirm it

## Why

The two weakest benchmarks were:

- **001_batch_commands** — 4 steps (efficiency 0.6667). The agent had all the info it needed (fixed paths, fixed values) but was likely doing an extra exploration or post-write verification step. The "batch" examples already existed in the prompt, but there was no explicit prohibition on these common time-wasting patterns.
- **004_source_reading** — 3 steps (efficiency 0.7778). Likely the agent reads the file first, then writes the answer separately, rather than using a single grep+awk pipeline as the example shows.

The existing system prompt told the agent what TO do (batch everything), but did not explicitly say what NOT to do. Adding a negative list of forbidden patterns closes that gap.

## Which benchmarks should improve

- **001_batch_commands**: should drop from 4 steps → 1–2 steps (efficiency 1.0 or 0.9, composite 1.0 or 0.98)
- **004_source_reading**: should drop from 3 steps → 1–2 steps (efficiency 1.0 or 0.9)

Overall `avg_efficiency` should improve from ~0.889 toward ~0.96+, pushing `avg_composite` from ~0.978 above the 1.02× threshold (~0.997).

## Diff

```diff
--- a/cmd/miniagent/agent/system.go
+++ b/cmd/miniagent/agent/system.go
@@ -27,6 +27,12 @@ Only issue a second bash call if the first one fails or if you genuinely need it
 Use &&, ||, ;, $(...), pipes, { ...; } grouping, and here-strings freely.
 Avoid separate tool calls for things that can be done together.
 
+## Anti-patterns that waste steps — NEVER do these
+- Reading a file back after writing it to verify the contents (trust the write succeeded)
+- Running an exploratory command (ls, cat, pwd) before the main task when the path is already given in the instructions
+- Splitting a single pipeline into multiple sequential bash calls
+- Writing a result and then issuing another call just to confirm it
+
 When the task is done, respond with a clear summary of what you accomplished.`
```
