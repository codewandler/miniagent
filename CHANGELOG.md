# Changelog

Entries are added automatically by the self-improvement loop (`task evolve`).
Each entry corresponds to one accepted improvement cycle.
No version numbers — revisions are synthetic counters local to this loop.

## revision 1 — 2026-04-15

Added explicit "Efficiency rules" to the system prompt and extended the bash tool description to instruct the model to batch multiple operations into a single command call using shell operators (&&, ||, ;, pipes, subshells). Benchmarks 001, 003, 004, and 005 are expected to drop from 2–3 steps to 1 step each, raising avg_composite from ~0.9734 toward 1.0000.

---
