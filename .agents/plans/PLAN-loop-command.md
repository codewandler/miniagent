# PLAN: add `/loop` for continuous repeated instruction execution

## Purpose

Add a REPL command:

```text
/loop <instruction>
```

that repeatedly runs the same instruction in a continuous agent loop until an explicit stop condition is triggered.

For the first version, the most practical stop condition should be a dedicated control command:

```text
/stop
```

with `Ctrl-C` remaining an emergency interrupt.

---

## Why this fits the current codebase

`miniagent` already has most of the primitives needed for this feature:

- persistent REPL sessions in `agent/repl.go`
- persistent conversation state across turns
- `/new` as an existing slash command
- per-turn cancellation via `context.CancelFunc`
- SIGINT handling already wired into the REPL
- `RunTurn(...)` as a reusable unit of work

So `/loop` is not a brand-new agent capability.
It is primarily a **REPL control-flow feature** layered on top of the current turn runner.

---

## Problem to solve

Right now the REPL can only execute one user instruction at a time.
If a user wants the agent to keep re-running the same instruction repeatedly, they must manually re-enter it after every completed turn.

That is awkward for tasks such as:

- “keep checking and fixing failing tests until I stop you”
- “keep reviewing the workspace for TODOs and resolve them one by one”
- “keep iterating on the benchmark until I tell you to stop”
- “continuously re-run this maintenance instruction while I monitor output”

The missing feature is a way to tell the REPL:

> keep issuing this instruction again and again until I explicitly stop the loop.

---

## Recommended v1 goal

Implement `/loop` as a **REPL-only command** that:

1. starts a background loop worker
2. repeatedly calls `a.RunTurn(...)` with the same instruction
3. keeps the REPL responsive to control commands
4. stops cleanly on `/stop`, `Ctrl-C`, or terminal errors

### Important design conclusion

If `/stop` must be usable while looping, then `/loop` cannot just be implemented as:

- parse `/loop ...`
- run `for { a.RunTurn(...) }` inline inside the scanner loop

because that would block REPL input and make `/stop` impossible to type.

So the correct design target is:

- loop execution in a goroutine
- REPL input remaining active for control commands

---

## Success criteria

This feature is successful when all of the following are true:

1. Typing `/loop <instruction>` starts repeated execution of that instruction.
2. The loop visibly reports iteration boundaries.
3. Typing `/stop` stops the loop without exiting the REPL.
4. `Ctrl-C` during an active loop cancels the current iteration and exits loop mode, but does not necessarily kill the whole process unless the user is already idle.
5. Only one loop can run at a time.
6. Normal REPL prompts do not race unsafe concurrent turns against the same `Agent` instance.
7. Existing non-loop REPL behavior still works: `exit`, `quit`, `/new`, single-turn tasks, EOF.
8. The feature is covered by deterministic tests.

---

## Non-goals for the first pass

Do **not** try to solve all of these in v1:

- multiple concurrent loops
- scheduled loops like “every 30s”
- rich loop condition syntax
- automatic semantic stop detection from model output
- background job management across process restarts
- a one-shot CLI mode like `miniagent loop ...`
- perfect terminal UI redraw behavior during concurrent output

The first version should focus on **safe repeated execution with explicit manual stopping**.

---

## Proposed user experience

## Core commands

### Start a loop

```text
/loop <instruction>
```

Example:

```text
/loop run the test suite, fix one failure, and continue improving the codebase
```

Behavior:

- starts loop mode
- stores the instruction as the active loop instruction
- immediately begins iteration 1
- after each completed turn, starts the next iteration automatically

### Stop the loop

```text
/stop
```

Behavior:

- if a loop is active, request stop
- if a turn is currently running, cancel it
- prevent any further iterations from starting
- return control to normal REPL mode

### Reset session

```text
/new
```

Recommended v1 behavior:

- if loop is active, reject `/new` with a clear message:
  - `Cannot reset while loop is active. Use /stop first.`
- once loop has stopped, `/new` behaves exactly as it does today

This avoids state races in the first version.

---

## Recommended v1 semantics

## One loop at a time

The REPL should allow at most one active loop.
If the user enters `/loop ...` while another loop is running, print a message such as:

```text
Loop already running. Use /stop first.
```

## Instruction reuse

Each iteration should submit the **same raw instruction string** to `RunTurn(...)`.

This keeps the feature simple and predictable.

## Session behavior

Recommended default for v1:

- reuse the current `Agent` session across iterations

Why:

- it matches current REPL semantics
- it allows the model to remember prior attempts
- it requires the least architectural disruption

Known downside:

- context will grow over time

That is acceptable for v1.
If context growth becomes a practical problem, a later version can add a reset policy such as:

- periodic `/new`-like reset every N iterations
- a loop mode that starts fresh each iteration

but that should not block the initial feature.

## Error behavior

Recommended v1 policy:

- if `RunTurn(...)` returns `nil`, continue looping
- if it returns `context.Canceled` because of `/stop` or `Ctrl-C`, stop looping cleanly
- if it returns `ErrMaxStepsReached`, stop the loop and report why
- if it returns any other error, stop the loop and report the error

Reasoning:

- infinite retry on real errors is expensive and surprising
- stopping on error is safer than thrashing the model and tools forever

## Control-input policy while loop is active

Recommended v1 rule:

- allow only control commands while a loop is active:
  - `/stop`
  - `exit`
  - `quit`
- reject normal free-text prompts with a message like:
  - `Loop is active. Use /stop before starting another task.`
- reject `/new` while active, as described above

This preserves agent safety by ensuring only one turn runs at a time.

---

## Stop conditions

## Explicit stop command

Primary stop condition for v1:

```text
/stop
```

This is the clearest interpretation of “until stop condition is called”.

## SIGINT / Ctrl-C

Current REPL behavior already distinguishes:

- mid-turn interrupt → cancel current turn
- idle interrupt → print session usage and exit process

With `/loop`, this should become:

- if loop is active and a turn is running:
  - cancel the current turn
  - mark the loop as stopping
- if loop is active but between iterations:
  - stop loop mode immediately
- if loop is not active and REPL is idle:
  - keep today’s exit behavior

## Safety guard

Even though `/loop` is intentionally open-ended, v1 should still consider a hidden or configurable safety cap such as:

- maximum iterations
- maximum total loop duration

This is optional for the first code change, but recommended as a follow-up safeguard if cost risk becomes a concern.

---

## Architecture proposal

## Add a loop controller

Introduce a small REPL-owned state object, for example:

```go
type loopController struct {
    mu          sync.Mutex
    active      bool
    stopping    bool
    instruction string
    iteration   int
    doneCh      chan loopResult
    cancel      context.CancelFunc
}

type loopResult struct {
    iteration int
    err       error
    stopped   bool
}
```

Exact shape can vary, but the controller should answer these questions:

- is a loop active?
- what instruction is being repeated?
- what iteration number is running?
- is stop requested?
- how do we cancel the current iteration?
- how do we notify the REPL that the loop ended?

## Add a dedicated worker goroutine

When `/loop <instruction>` is entered:

1. validate no loop is already active
2. initialize loop state
3. start a goroutine that repeatedly:
   - increments iteration counter
   - creates a turn context
   - registers its cancel func with the controller
   - prints a loop banner such as `Loop iteration 3`
   - calls `a.RunTurn(...)`
   - decides whether to continue or stop
4. notify the REPL when the worker exits

## Keep REPL input alive

The REPL must remain able to read user input while the loop worker is running.
That means `RunREPL(...)` should evolve from a purely serial scanner loop into a small coordinator that handles:

- scanner input
- loop completion notifications
- signal-driven cancellation state

It does **not** need to become a full TUI.
It just needs enough structure to safely manage background loop execution.

---

## REPL state-machine target

A simple conceptual state machine is enough.

### Idle

- no loop active
- normal prompts accepted
- `/loop` can start a loop
- `/new` works
- `exit`/`quit` exits

### LoopRunning

- loop worker exists
- either a turn is currently running or the worker is about to start the next one
- normal prompts are rejected
- `/stop` is accepted
- `/new` is rejected

### LoopStopping

- stop has been requested
- current turn may still be unwinding
- no new iterations may start
- once worker exits, transition back to Idle

This keeps the implementation easy to reason about.

---

## Integration with current code

## Likely files to change

### `agent/repl.go`

Main implementation site.
Expected changes:

- parse `/loop ` commands
- parse `/stop`
- add loop controller state
- adjust signal handling to respect loop state
- keep session usage printing on final exit

### `agent/repl_test.go`

Primary test coverage site.
Add tests for loop lifecycle and control commands.

### Possible new file: `agent/loop.go`

If `repl.go` becomes crowded, extract loop-specific controller logic here.
This is likely cleaner than inflating `repl.go` with synchronization details.

### `README.md`

Document new REPL commands.
Add a small example showing `/loop` and `/stop`.

### `CHANGELOG.md`

Record the feature.

---

## Recommended implementation phases

# Phase 1 — Define exact UX and state rules

Lock down the first-pass command contract:

- `/loop <instruction>` starts loop mode
- `/stop` stops loop mode
- only one loop at a time
- normal prompts blocked while loop active
- `/new` blocked while loop active
- stop on errors rather than retry forever

## Deliverable

A minimal design note in code comments or this plan translated into implementation decisions.

---

# Phase 2 — Add loop controller and background execution

Implement loop state and worker orchestration.

## Work items

- create controller struct
- add worker lifecycle methods: start, requestStop, cancelCurrent, finish
- ensure no concurrent `RunTurn(...)` calls happen on the same `Agent`
- print clear iteration markers

## Deliverable

A running `/loop` feature that can start and repeat turns.

---

# Phase 3 — Integrate `/stop` and SIGINT behavior

Make stopping reliable and intuitive.

## Work items

- add `/stop` parsing
- cancel current turn if needed
- prevent next iteration from starting
- update SIGINT behavior to stop the loop rather than unexpectedly exiting the whole process

## Deliverable

A loop that can be stopped both interactively and via `Ctrl-C`.

---

# Phase 4 — Harden output and edge cases

Improve user clarity.

## Work items

- print status when loop starts and stops
- print why the loop stopped:
  - user requested stop
  - interrupted
  - max steps reached
  - turn error
- reject invalid `/loop` usage such as empty instruction
- reject second `/loop` while active
- reject normal prompts while active

## Deliverable

A predictable, understandable REPL experience.

---

# Phase 5 — Tests and docs

Add deterministic coverage and public documentation.

## Deliverable

- unit tests in `agent/repl_test.go`
- README slash-command documentation
- CHANGELOG entry

---

## Test plan

## Core tests

### `/loop` requires an instruction

Input:

```text
/loop
exit
```

Expect:

- helpful usage message
- no turn executed

### `/loop` starts repeated execution

Input conceptually:

```text
/loop say hello
/stop
exit
```

Expect:

- loop start message
- at least one iteration banner
- at least one `RunTurn(...)` execution
- clean stop message

### second `/loop` is rejected

Input:

```text
/loop task one
/loop task two
/stop
exit
```

Expect:

- second loop start rejected
- original loop remains authoritative

### normal prompt during active loop is rejected

Input:

```text
/loop keep working
what is the current status?
/stop
exit
```

Expect:

- free-text prompt rejected while loop active
- no concurrent second turn

### `/new` during active loop is rejected

Input:

```text
/loop keep working
/new
/stop
exit
```

Expect:

- reset refused until stop

### `Ctrl-C` stops loop mode

Using a controllable fake or cancellable test double:

- start loop
- interrupt active turn
- confirm loop exits rather than immediately starting another iteration

### loop stops on non-cancel error

Using a fake client that returns an error:

- start loop
- first iteration fails
- loop reports the error and stops

## Testing note

The current REPL tests are mostly linear text-input tests.
For `/loop`, deterministic tests will likely need:

- a fake agent/runtime whose turn execution can block until released
- synchronization channels to observe iteration start/stop
- assertions that no concurrent turn execution happens

This is worth doing carefully because loop bugs are usually race bugs.

---

## Risks and mitigations

## Risk: `/stop` is impossible with a blocking implementation

### Mitigation

Run the loop in a goroutine and keep input scanning alive.

## Risk: concurrent `RunTurn(...)` calls corrupt session state

### Mitigation

Permit only one active turn at a time.
Reject normal prompts while loop mode is active.

## Risk: `Ctrl-C` semantics become confusing

### Mitigation

Make loop-aware signal rules explicit and test them.
Prefer “stop loop first” over “exit process immediately” when looping.

## Risk: output becomes messy because background loop and prompt share stdout

### Mitigation

Accept modest interleaving in v1.
Print explicit banners for start/stop/iteration.
Avoid trying to build a full-screen terminal UI.

## Risk: runaway token/tool cost

### Mitigation

Stop on errors.
Consider a future hard cap on total iterations or duration.
Make loop status visible.

---

## Future extensions after v1

These should only come after the basic loop is stable:

- `/loop status`
- `/loop stop` as an alias for `/stop`
- configurable delay between iterations
- `--max-iterations` or REPL-configurable loop caps
- `--stop-on-error=false` retry mode
- periodic session reset for long-running loops
- condition-based stopping, for example:
  - stop when tests pass
  - stop when no files changed
  - stop when assistant emits a marker

But these are follow-ups, not part of the first implementation target.

---

## Recommended final scope

The right first implementation is:

- REPL-only
- one active loop max
- `/loop <instruction>` to start
- `/stop` to end
- `Ctrl-C` to interrupt safely
- reuse current session across iterations
- stop on any non-cancel error
- block other prompts while loop is active

That gives `miniagent` a practical continuous-operation mode without overcomplicating the initial change.
