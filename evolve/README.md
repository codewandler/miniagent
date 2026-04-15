# miniagent — evolution artefacts

This directory is managed by `evolve.sh`. Do not edit manually.

## Structure

```
evolve/
├── README.md           this file
├── cycles.jsonl        one JSON line per completed cycle (score history)
├── bin/
│   ├── miniagent-stable   current best binary (updated each kept cycle)
│   └── stable_commit      git SHA the stable binary was compiled from
├── scores/
│   ├── cycle_N_stable_BENCH.json      per-benchmark result, stable
│   ├── cycle_N_candidate_BENCH.json   per-benchmark result, candidate
│   ├── cycle_N_stable_aggregate.json  aggregated scores, stable
│   └── cycle_N_candidate_aggregate.json
└── reasoning/
    └── cycle_N.md    what the agent changed and why (written by the agent)
```

## How a cycle works

```
1. BASELINE    run all benchmarks/ with the current stable binary
2. REASON      stable agent reads its own source + scores, proposes ONE change
3. IMPLEMENT   stable agent edits agent/*.go or main.go, verifies go build
4. BUILD       compile candidate binary (auto-reverts on build failure)
5. BENCHMARK   run all benchmarks/ with the candidate binary
6. JUDGE       candidate_score > stable_score × 1.02 → KEEP, else REVERT
7a. KEEP       git commit + git push, candidate becomes new stable
7b. REVERT     git restore source files, try again next cycle
```

## Scoring

```
composite  = completed×0.40 + correct×0.40 + efficiency×0.20
efficiency = clamp(1 − steps÷MAX_BENCH_STEPS, 0, 1)
```

A candidate must score **> stable × 1.02** to be accepted.
