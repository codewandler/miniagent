# SWE-bench Integration Plan

## Objective

Integrate a **real repository-level external benchmark** into miniagent by adding a **SWE-bench track** alongside the current toy internal benchmarks.

The goal is **not** to replace the existing benchmark loop immediately. The goal is to create a second evaluation path that:

- measures real bug-fixing ability on real repositories
- stays reproducible enough to use inside the self-improvement loop
- starts small and cheap
- can later become a primary optimization target

---

## Success criteria

This integration is successful when all of the following are true:

1. `miniagent` can be run against a curated SWE-bench instance in an isolated workspace.
2. The run produces a transcript, patch diff, and machine-readable result JSON.
3. Correctness is judged by test outcomes or official-compatible validation, not by `/tmp/bench_result.txt`.
4. A small curated subset can run repeatedly with acceptable flake rate.
5. Stable and candidate binaries can be compared on that subset.
6. The self-improvement loop can optionally use SWE results as part of acceptance.

---

## Non-goals

At least initially, this plan does **not** aim to:

- run full SWE-bench on every cycle
- perfectly reproduce the entire official evaluation stack on day 1
- replace the current `benchmarks/*.md` format
- make SWE-bench the only acceptance criterion immediately
- solve all environment reproducibility problems up front

---

## Current-state diagnosis

The current benchmark system is optimized for very small shell tasks:

- benchmark definition: markdown files under `benchmarks/`
- success criterion: process exit code and optional expected string in `/tmp/bench_result.txt`
- efficiency metric: tool-call count
- aggregate score: completion + correctness + efficiency

This setup is good for validating:

- command batching
- shell fluency
- simple source inspection
- file manipulation behavior

It is **not** good for validating:

- issue understanding
- repo exploration
- selective test execution
- bug localization
- multi-file patching
- regression avoidance
- real software engineering outcomes

That mismatch is why the score has saturated.

---

## High-level strategy

Use a **parallel benchmark pipeline**.

- Keep internal benchmarks as fast sanity checks.
- Add a separate SWE-bench runner for real-world evaluation.
- Start with a curated subset of SWE-bench Lite or Verified.
- Make SWE report-only first.
- Later introduce dual scoring and then combined acceptance.

This avoids destabilizing the existing loop while giving the agent something real to optimize toward.

---

## Design principles

1. **Separate concerns**  
   Do not force SWE-bench into the current markdown task format.

2. **Small first**  
   Start with 1 instance, then 5, then 10–20.

3. **Normalized metadata**  
   Convert external dataset records into a repo-local schema.

4. **Reproducibility over breadth**  
   Prefer a tiny stable subset over a larger flaky one.

5. **Resolved rate over efficiency**  
   Real correctness must dominate scoring.

6. **Cheap inner loop, expensive outer loop**  
   Use tiny SWE subsets in evolution cycles; run broader evaluation only on demand.

---

# Phase 0 — Scope and benchmark policy

## Decision: what to integrate first

Recommended starting point:

- dataset family: **SWE-bench Lite** first
- backup option: small curated subset of **SWE-bench Verified**
- instance count for first useful version: **5 instances**
- instance count for initial bring-up: **1 instance**

## Why Lite first

- cheaper to run
- simpler to debug
- lower infrastructure burden
- sufficient to validate the runner architecture

## Initial benchmark policy

For the first version:

- report **resolved_rate** as the headline metric
- also report `completion_rate`, `avg_steps`, `avg_duration_s`
- do not use the internal benchmark scoring formula unchanged
- do not gate acceptance on SWE results yet

## Deliverable

A written benchmark policy in the repo, likely under:

```text
external_bench/swebench/README.md
```

That document should define:

- which SWE subset is being used
- how instances are selected
- what “resolved” means in this repo
- how often it is run
- whether results are official-comparable or only locally comparable

---

# Phase 1 — Filesystem layout and repo structure

## Proposed layout

```text
.agents/plans/
external_bench/
  swebench/
    README.md
    instances/
      curated_lite.txt
      curated_verified.txt
      normalized/
    prompts/
      system_appendix.txt
    scripts/
      fetch_dataset.sh
      normalize_instances.sh
      prepare_instance.sh
      run_agent_on_instance.sh
      evaluate_instance.sh
      aggregate_results.sh
      bench_swe.sh
    cache/
      datasets/
      repos/
      envs/
    logs/
    results/
```

## Rationale

- `instances/` stores selected and normalized instance metadata
- `scripts/` contains the benchmark runner pipeline
- `cache/` avoids expensive repeated setup
- `logs/` stores transcripts and validation output
- `results/` stores machine-readable per-run artifacts

## Deliverable

Create directory layout and a minimal README explaining the pipeline.

---

# Phase 2 — Normalized instance schema

## Problem

Raw SWE-bench data structures are external and can change. The rest of this repo should not depend directly on them.

## Solution

Introduce a normalized per-instance JSON format.

## Required fields

Each normalized instance should contain:

- `instance_id`
- `dataset_split`
- `repo`
- `base_commit`
- `issue_text`
- `hints_text` if available
- `test_command` or evaluation metadata
- `setup_strategy`
- `timeout_sec`
- `language`

## Suggested schema

```json
{
  "instance_id": "sympy__sympy-12345",
  "dataset_split": "lite",
  "repo": "sympy/sympy",
  "base_commit": "abc123",
  "issue_text": "GitHub issue text...",
  "hints_text": "Optional hints...",
  "test_command": "python -m pytest path/to/test",
  "setup_strategy": "python-pytest",
  "timeout_sec": 1800,
  "language": "python"
}
```

## Normalization script responsibilities

`normalize_instances.sh` should:

1. fetch or read raw SWE metadata
2. project only the fields miniagent needs
3. write one normalized JSON per instance
4. create curated lists referencing normalized files

## Deliverable

At least one normalized instance checked into the repo or generated deterministically by script.

---

# Phase 3 — Benchmark runner contract

## New command surface

Add a dedicated command path, for example:

- `task bench:swe`
- or `external_bench/swebench/scripts/bench_swe.sh`

## Required behavior

The runner must:

1. select instance list
2. prepare isolated workspace
3. run miniagent on issue text
4. capture logs and diff
5. run validation
6. write per-instance JSON
7. aggregate results

## Important separation

This runner should be **independent** from the markdown benchmark runner in `evolve.sh`, even if both later feed into the same evolution summary.

## Deliverable

A single script entrypoint that can run one or more normalized instances.

---

# Phase 4 — Instance preparation workflow

## Required preparation steps

For each instance:

1. ensure raw repo exists in cache
2. create isolated worktree or copied workspace
3. checkout the buggy/base commit
4. apply any environment setup logic
5. record prep logs
6. optionally verify baseline state

## Preferred isolation model

Short term:

- git worktree or temp copy

Longer term:

- per-instance container execution

## Why not copy the whole official harness immediately

Because the first objective is to prove the pipeline works in this repo with manageable complexity.

## Deliverable

`prepare_instance.sh` that outputs:

- workspace path
- resolved instance metadata path
- prep log path

---

# Phase 5 — Agent invocation profile

## Problem

The current system prompt is strongly optimized for one-shot bash batching. That is helpful for toy shell tasks, but incomplete for repo-level bug fixing.

## Recommendation

Add a SWE-bench-specific prompt appendix or profile.

## Prompt goals

The SWE profile should encourage the agent to:

- inspect repo structure before editing
- identify relevant tests quickly
- run targeted tests before broad test suites
- make the smallest plausible fix
- verify the fix before stopping
- avoid broad speculative refactors
- prefer evidence-driven edits

## Possible interface

Near-term:

- inject extra prompt text only from the SWE runner

Longer-term:

```bash
miniagent --profile swebench ...
```

## Deliverable

`external_bench/swebench/prompts/system_appendix.txt`

---

# Phase 6 — Agent run capture

## After each agent run, capture:

- transcript/log file
- exit code
- duration
- tool-call count
- `git diff`
- changed file list

## Why this matters

You need these artifacts for:

- debugging failures
- measuring completion vs resolution
- studying agent behavior changes across cycles
- future qualitative analysis

## Deliverable

`run_agent_on_instance.sh` writes a complete artifact bundle per instance.

Suggested result bundle:

```text
results/<run_id>/<instance_id>/
  instance.json
  run.log
  eval.log
  diff.patch
  changed_files.txt
  result.json
```

---

# Phase 7 — Evaluation semantics

## Primary rule

Correctness must be determined by repository behavior, not by text output.

## Preferred evaluation order

1. official-compatible SWE evaluation where feasible
2. instance-specific test command from normalized metadata
3. explicit local validator wrapper if neither of the above is practical

## Per-instance result schema

```json
{
  "instance_id": "repo__issue",
  "label": "stable",
  "cycle": 3,
  "completed": true,
  "resolved": false,
  "steps": 18,
  "duration_s": 742,
  "files_changed": 4,
  "exit_code": 0,
  "workspace": "/tmp/...",
  "diff_path": "results/.../diff.patch",
  "log_path": "results/.../run.log"
}
```

## Aggregate schema

```json
{
  "label": "stable",
  "cycle": 3,
  "n": 5,
  "resolved_rate": 0.2,
  "completion_rate": 0.8,
  "avg_steps": 21.4,
  "avg_duration_s": 655.1,
  "instances": ["..."]
}
```

## Deliverable

`evaluate_instance.sh` and `aggregate_results.sh`

---

# Phase 8 — Scoring policy

## Recommended SWE scoring

### Headline metric

- `resolved_rate`

### Supporting metrics

- `completion_rate`
- `avg_steps`
- `avg_duration_s`
- optional `avg_files_changed`

## What not to do

Do not weight efficiency heavily enough that a fast but incorrect run looks good.

## If a scalar score is required

Use:

```text
swe_score = 0.8 * resolved + 0.2 * completed
```

But keep the raw resolved rate visible and primary.

## Deliverable

Document the scoring policy in `external_bench/swebench/README.md`.

---

# Phase 9 — Evolution-loop integration strategy

## Stage 1 — Report-only integration

Add optional SWE evaluation to the loop, but do not use it for acceptance.

### Purpose

- validate pipeline stability
- characterize runtime
- collect baselines
- avoid derailing current self-improvement flow

## Stage 2 — Dual scorecards

Record both:

- internal benchmark score
- SWE resolved rate

Suggested acceptance rule:

```text
accept if:
  internal_score >= stable_internal_score
  and swe_resolved_rate > stable_swe_resolved_rate
```

## Stage 3 — Combined acceptance

Once stable:

```text
combined_score = 0.4 * internal_score + 0.6 * swe_resolved_rate
```

Accept only if combined meaningfully improves.

## Noise warning

Do not reuse the current `1.02x` threshold blindly on very small SWE subsets.

## Deliverable

A configurable flag in `evolve.sh`, something like:

- `SWE_ENABLED=1`
- `SWE_REPORT_ONLY=1`

---

# Phase 10 — Concrete `evolve.sh` changes

## Add config knobs

Introduce variables such as:

```bash
SWEBENCH_DIR="$WORKSPACE/external_bench/swebench"
SWE_ENABLED="${SWE_ENABLED:-0}"
SWE_REPORT_ONLY="${SWE_REPORT_ONLY:-1}"
SWE_INSTANCE_LIST="${SWE_INSTANCE_LIST:-$SWEBENCH_DIR/instances/curated_lite.txt}"
SWE_MAX_STEPS="${SWE_MAX_STEPS:-50}"
SWE_CMD_TIMEOUT="${SWE_CMD_TIMEOUT:-120}"
SWE_INSTANCE_TIMEOUT="${SWE_INSTANCE_TIMEOUT:-1800}"
SWE_RESULTS_DIR="$SWEBENCH_DIR/results"
```

## Add functions

Suggested new shell functions:

- `run_one_swe_instance`
- `run_all_swe_instances`
- `maybe_run_swe_baseline`
- `maybe_run_swe_candidate`
- `summarize_swe_results`

## Integration points

Add optional SWE runs after:

1. stable build benchmark stage
2. candidate benchmark stage

Do **not** interleave SWE execution with the existing markdown benchmark loop.

## Deliverable

A minimal patch plan for `evolve.sh` that only adds optional SWE stages and does not alter the existing benchmark path.

---

# Phase 11 — Subset selection policy

## Selection criteria for first instances

Choose instances that are:

- stable to set up
- moderate in runtime
- easy to evaluate
- not giant monorepos
- not multi-service or highly stateful
- likely solvable with current miniagent capabilities

## Suggested progression

### Bring-up set
- 1 instance

### Initial useful set
- 5 instances: 3 easy, 2 medium

### First serious set
- 10–20 instances after flake rate is acceptable

## Deliverable

A checked-in curated list with a brief rationale per instance.

---

# Phase 12 — Reproducibility and flake control

## Known risk sources

- dependency installation failures
- repo setup divergence
- hidden network assumptions
- long-running tests
- environment contamination between runs

## Mitigations

1. cache repos locally
2. cache environment setup where possible
3. prefer isolated worktrees or temp dirs
4. move to containerized execution when the basic pipeline works
5. capture prep logs separately from agent logs
6. enforce strict timeout behavior
7. keep the first subset tiny and stable

## Deliverable

A short flake policy document in the SWE bench README.

---

# Phase 13 — Milestones and acceptance criteria

## Milestone 1 — Skeleton

### Scope
- directory layout exists
- README exists
- entry script exists
- instance list loading works
- dummy JSON output works

### Acceptance
- `task bench:swe` runs successfully without real evaluation logic yet

## Milestone 2 — One real instance

### Scope
- one normalized instance
- workspace preparation works
- miniagent runs
- diff/log/result captured
- validation runs

### Acceptance
- one instance can be executed end-to-end reproducibly

## Milestone 3 — Five-instance subset

### Scope
- curated 5-instance set
- aggregate results work
- basic caching works

### Acceptance
- subset produces stable aggregate output with inspectable artifacts

## Milestone 4 — Loop visibility

### Scope
- stable and candidate can both run SWE subset
- summaries appear in evolution output

### Acceptance
- one full evolution cycle can report both internal and SWE metrics

## Milestone 5 — Optional acceptance gating

### Scope
- config-controlled SWE-aware acceptance rule

### Acceptance
- loop can be switched between report-only and dual-score modes without breaking existing behavior

---

# Phase 14 — Work breakdown by file

## New files likely needed

```text
external_bench/swebench/README.md
external_bench/swebench/instances/curated_lite.txt
external_bench/swebench/prompts/system_appendix.txt
external_bench/swebench/scripts/bench_swe.sh
external_bench/swebench/scripts/fetch_dataset.sh
external_bench/swebench/scripts/normalize_instances.sh
external_bench/swebench/scripts/prepare_instance.sh
external_bench/swebench/scripts/run_agent_on_instance.sh
external_bench/swebench/scripts/evaluate_instance.sh
external_bench/swebench/scripts/aggregate_results.sh
```

## Existing files likely touched

```text
Taskfile.yaml
AGENTS.md
evolve/README.md
evolve.sh
.gitignore
agent/system.go   # maybe, if a profile or prompt appendix is added
main.go           # maybe, if a benchmark/profile flag is added
```

---

# Phase 15 — Recommended implementation order

## Week 1

1. add directory layout
2. write SWE README and policy
3. define normalized schema
4. add runner skeleton
5. support one manual instance

## Week 2

1. capture diff/log/result bundles
2. add real evaluation for one instance
3. add aggregate results
4. create curated list of 5 instances

## Week 3

1. add caching and prep logs
2. run 5-instance subset repeatedly
3. reduce flakiness
4. add report-only integration to `evolve.sh`

## Week 4

1. add optional dual-score acceptance
2. refine prompt/profile for SWE runs
3. expand curated subset if stable

---

# Repo-specific recommendations

## Recommendation 1
Keep the current internal benchmarks permanently.

They are still useful as:

- fast regression tests for shell behavior
- cheap sanity checks during prompt evolution
- guardrails against making the terminal agent clumsy

## Recommendation 2
Do not overfit the whole project to full official SWE-bench infra too early.

First prove:

- you can run one instance well
- artifacts are captured properly
- stable vs candidate comparisons are meaningful

## Recommendation 3
Treat SWE as the “outer-loop truth” and internal benchmarks as “inner-loop hygiene”.

That division fits this repo well.

---

# Final recommendation

The best integration path is:

1. build a **separate SWE-bench runner**
2. start with **1 real instance**, then **5 curated instances**
3. use a **normalized instance schema**
4. judge correctness by **repo validation**, not text output
5. keep SWE **report-only** at first
6. only later make SWE part of candidate acceptance

That gives miniagent a real software-engineering target without destabilizing the current evolution loop.

---

## Progress update — implemented milestone snapshot

Implemented in the repo now:

- A dockerized SWE benchmark command via `task bench:swe`
- A working artifact pipeline under `external_bench/swebench/`
- Real normalized SWE-bench Lite Django instances:
  - `django__django-10914`
  - `django__django-10924`
  - `django__django-11179`
- A Go-centric benchmark track using the same harness contract:
  - `gin__joinpaths-0001`
  - `gin__parseaccept-0002`
  - `gin__filterflags-0003`
- Real result bundles with:
  - `prep.log`
  - `run.log`
  - `eval.log`
  - `diff.patch`
  - `result.json`
  - `aggregate.json`

At this point the harness supports two useful benchmark sets:

1. `curated_real.txt` for real SWE-bench Lite instances
2. `curated_go.txt` for fast Go-centric external-style tasks

The next major step is agent-driven execution on these prepared repos, followed by report-only integration into `evolve.sh`.
