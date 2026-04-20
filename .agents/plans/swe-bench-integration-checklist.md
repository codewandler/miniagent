# SWE-bench Integration Checklist

## Milestone 1 — Skeleton
- [x] Create `external_bench/swebench/` directory layout
- [x] Add `external_bench/swebench/README.md`
- [x] Add a minimal normalized instance example
- [x] Add script skeletons under `external_bench/swebench/scripts/`
- [x] Add `bench_swe.sh` entrypoint
- [x] Make the runner emit placeholder per-instance and aggregate JSON
- [x] Add a `Taskfile.yaml` entry for `bench:swe`
- [x] Document whether results are placeholder or real

## Milestone 2 — Real docker-runnable benchmark
- [x] Pick one initial bring-up instance
- [x] Implement isolated workspace preparation
- [x] Capture transcript, diff, changed files, exit code, and duration
- [x] Run real validation command
- [x] Write real per-instance result JSON
- [x] Make the benchmark runnable via Docker
- [x] Produce aggregate JSON from a real run
- [x] Add one real SWE-bench Lite instance
- [x] Add two more real SWE-bench Lite instances

## Milestone 3 — Go-centric benchmark track
- [x] Add `swebench-go` setup strategy
- [x] Add one Go-centric external-style benchmark
- [x] Expand Go subset to 3 runnable benchmarks
- [x] Verify curated Go subset runs end-to-end in Docker
- [ ] Replace Go fixtures with official external Go instances where practical

## Milestone 4 — Agent-driven runs
- [ ] Invoke `miniagent` successfully in `agent` mode on the prepared real repos
- [ ] Tune agent-facing prompts for repo repair loops
- [ ] Capture and compare oracle vs agent benchmark outcomes

## Milestone 5 — Report-only loop integration
- [ ] Add optional SWE config knobs to `evolve.sh`
- [ ] Run stable binary on SWE subset
- [ ] Run candidate binary on SWE subset
- [ ] Print SWE summary alongside internal benchmark summary
- [ ] Keep acceptance logic unchanged initially

## Milestone 6 — Acceptance integration
- [ ] Add dual-score acceptance mode
- [ ] Add configurable report-only vs gating mode
- [ ] Tune thresholding for small SWE subsets
- [ ] Document the new acceptance policy

## Current runnable benchmark sets
- `external_bench/swebench/instances/curated_real.txt` — 3 real SWE-bench Lite Django instances
- `external_bench/swebench/instances/curated_go.txt` — 3 Go-centric external-style benchmark instances
