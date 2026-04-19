# SWE-bench integration scaffold

This directory contains a working, docker-runnable benchmark pipeline for a **SWE-bench-style** task, plus the structure needed to grow into real SWE-bench integration.

## Current status

Implemented now:
- docker-runnable benchmark task: `task bench:swe`
- normalized instance metadata
- isolated workspace preparation
- artifact capture per instance
- real validation using repository tests
- aggregate result JSON
- two execution drivers:
  - `oracle` (default): applies a checked-in gold patch for deterministic bring-up
  - `agent`: runs `miniagent` against the issue prompt

Still pending for full SWE-bench integration:
- fetching official SWE-bench data
- normalization from official dataset records
- curated external instance subsets
- official-compatible evaluation semantics
- report-only / gated integration into `evolve.sh`

## What is benchmarked today

The current runnable instance is a **local demo fixture** shaped like a SWE-bench task:
- a repository fixture with a real bug
- a natural-language issue description
- validation by `go test ./...`
- a recorded gold patch for deterministic runs

This proves the end-to-end harness works in Docker and produces actual results.

## Running

Deterministic oracle run:

```bash
task bench:swe
```

Agent-driven run (requires model credentials configured for `miniagent`):

```bash
SWE_DRIVER=agent task bench:swe
```

## Output layout

Each run writes artifacts under:

```text
external_bench/swebench/results/<run_id>/
  aggregate.json
  <instance_id>/
    instance.json
    prep.log
    run.log
    eval.log
    diff.patch
    changed_files.txt
    result.json
    workspace/
```

## Result semantics

Per-instance results report:
- `completed`: driver step completed successfully
- `resolved`: validation command succeeded
- `steps`: tool-call count when using the `agent` driver, or 1 for `oracle`
- `duration_s`: driver runtime in seconds

Aggregate results report:
- `resolved_rate`
- `completion_rate`
- `avg_steps`
- `avg_duration_s`

## Next step toward real SWE-bench

Replace the local demo instance with normalized records derived from real SWE-bench Lite / Verified instances while keeping the same runner contract.


## Real instance bring-up

A first normalized real SWE-bench Lite instance is available in `instances/curated_real.txt` (`django__django-10914`).
Run it with:

```bash
SWE_DRIVER=oracle task bench:swe -- external_bench/swebench/instances/curated_real.txt
```
