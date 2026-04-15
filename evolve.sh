#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════════════
# evolve.sh — miniagent self-improvement loop
#
# Runs INSIDE the hardened Docker container launched by `task evolve`.
# Each cycle:
#   1. SETUP      ensure a stable binary exists (bootstraps from image on first run)
#   2. BASELINE   benchmark the stable binary → numeric scores
#   3. REASON     stable agent reads its own source + scores, proposes ONE change
#   4. IMPLEMENT  stable agent edits agent/*.go / main.go, verifies go build
#   5. BUILD      compile candidate binary
#   6. BENCHMARK  run the same suite against the candidate
#   7. JUDGE      compare scores (pure arithmetic — no LLM bias)
#   8a. KEEP      git commit, promote candidate → new stable, continue
#   8b. REVERT    git restore source, discard candidate, continue
#
# Usage (via task):
#   task evolve                   # infinite loop
#   task evolve -- --cycles 3    # stop after 3 cycles
#   task evolve -- --dry-run     # show what would run, skip LLM calls
# ═══════════════════════════════════════════════════════════════════════════
set -euo pipefail

WORKSPACE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$WORKSPACE/../.." && pwd)"

EVOLVE_DIR="$WORKSPACE/evolve"
BENCHMARKS_DIR="$WORKSPACE/benchmarks"
SCORES_DIR="$EVOLVE_DIR/scores"
REASONING_DIR="$EVOLVE_DIR/reasoning"
BIN_DIR="$EVOLVE_DIR/bin"
REVISION_FILE="$EVOLVE_DIR/revision"

# Stable binary lives in the bind-mounted repo (survives container restarts).
# Bootstrapped from the image-baked binary on first run.
STABLE_BIN="$BIN_DIR/miniagent-stable"
IMAGE_BIN="/usr/local/bin/miniagent"

# Candidate is compiled into the bind-mounted path (exec works; gitignored).
CANDIDATE_BIN="$BIN_DIR/miniagent-candidate"

# ── Configuration ──────────────────────────────────────────────────────────
MAX_CYCLES=0          # 0 = infinite
DRY_RUN=false
MAX_BENCH_STEPS=10    # cap tool-call iterations during benchmarking
BENCH_CMD_TIMEOUT=30  # per-command timeout (seconds) inside each benchmark

# Score weights (must sum to 1.0)
W_COMPLETED=0.40   # did the agent exit 0?
W_CORRECT=0.40     # did /tmp/bench_result.txt match the expected string?
W_EFFICIENCY=0.20  # fewer steps = higher score

# Accept a candidate only if it beats stable by at least this factor
IMPROVEMENT_THRESHOLD=1.02

# ── Colours (all output goes to stderr so $() captures only return values) ─
RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m'
BLUE='\033[0;34m' CYAN='\033[0;36m' BOLD='\033[1m' RESET='\033[0m'

log()    { echo -e "${BLUE}[$(date '+%H:%M:%S')] $*${RESET}" >&2; }
ok()     { echo -e "${GREEN}[$(date '+%H:%M:%S')] ✓ $*${RESET}" >&2; }
warn()   { echo -e "${YELLOW}[$(date '+%H:%M:%S')] ⚠ $*${RESET}" >&2; }
fail()   { echo -e "${RED}[$(date '+%H:%M:%S')] ✗ $*${RESET}" >&2; }
header() { echo -e "\n${BOLD}${CYAN}$*${RESET}" >&2; }

# ── Arg parsing ────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --cycles)   MAX_CYCLES="$2"; shift 2 ;;
    --dry-run)  DRY_RUN=true; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done

# ── Ensure required tools ──────────────────────────────────────────────────
for tool in jq git go bc awk; do
  command -v "$tool" &>/dev/null || { fail "Required tool not found: $tool"; exit 1; }
done

# ── Directory setup ────────────────────────────────────────────────────────
mkdir -p "$BIN_DIR" "$SCORES_DIR" "$REASONING_DIR" "$BENCHMARKS_DIR"
mkdir -p "${GOCACHE:-/repo/.go-cache/build}" "${GOMODCACHE:-/repo/.go-cache/mod}"

# ── Git identity (works inside container without host ~/.gitconfig) ─────────
export GIT_AUTHOR_NAME="${GIT_AUTHOR_NAME:-miniagent}"
export GIT_AUTHOR_EMAIL="${GIT_AUTHOR_EMAIL:-miniagent@evolve.local}"
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"

# ═══════════════════════════════════════════════════════════════════════════
# PHASE 0 — SETUP
# ═══════════════════════════════════════════════════════════════════════════
phase_setup() {
  # Allow git to operate on the repo even when the container UID differs
  # from the repo owner (git 2.35.2+ safe.directory check).
  git config --global --add safe.directory "$REPO_ROOT" 2>/dev/null || true

  # Load (or initialise) the revision counter
  CURRENT_REVISION=$(cat "$REVISION_FILE" 2>/dev/null || echo 0)

  if [[ ! -f "$STABLE_BIN" ]]; then
    log "First run — bootstrapping stable binary from image binary..."
    cp "$IMAGE_BIN" "$STABLE_BIN"
    chmod +x "$STABLE_BIN"
    local commit; commit=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo "unknown")
    echo "$commit" > "$BIN_DIR/stable_commit"
    ok "Stable binary ready: $STABLE_BIN  (commit: $commit)"
  else
    local commit; commit=$(cat "$BIN_DIR/stable_commit" 2>/dev/null || echo "unknown")
    ok "Using existing stable binary (commit: $commit)"
  fi
}

# ═══════════════════════════════════════════════════════════════════════════
# PHASE 1 / 5 — BENCHMARK
# Runs all benchmarks/*.md against a binary, writes per-benchmark JSON scores.
# Prints the aggregate composite score to stdout (only stdout output).
# ═══════════════════════════════════════════════════════════════════════════
run_one_benchmark() {
  local binary="$1" label="$2" cycle="$3" bench_file="$4"
  local bench_name; bench_name=$(basename "$bench_file" .md)
  local result_file="$SCORES_DIR/cycle_${cycle}_${label}_${bench_name}.json"
  local log_file="$SCORES_DIR/cycle_${cycle}_${label}_${bench_name}.log"

  # Pull the EXPECTED: line (if any) — string that must appear in /tmp/bench_result.txt
  local expected; expected=$(grep "^EXPECTED:" "$bench_file" | head -1 | sed 's/^EXPECTED:[[:space:]]*//' || true)
  local task_content; task_content=$(cat "$bench_file")

  rm -f /tmp/bench_result.txt

  local t0; t0=$(date +%s)
  set +e
  timeout 180 "$binary" \
    --workspace "$REPO_ROOT" \
    --max-steps  "$MAX_BENCH_STEPS" \
    --timeout    "${BENCH_CMD_TIMEOUT}s" \
    "$task_content" \
    >"$log_file" 2>&1
  local exit_code=$?
  set -e
  local duration=$(( $(date +%s) - t0 ))

  # Count tool calls — awk always exits 0 (avoids pipefail from grep exit-1)
  local steps; steps=$(awk '/🔧/{c++} END{print c+0}' "$log_file" 2>/dev/null || echo 0)

  # completed: exited 0
  local completed=0
  [[ $exit_code -eq 0 ]] && completed=1

  # correct: /tmp/bench_result.txt contains the expected string
  local correct=0
  if [[ -n "$expected" ]]; then
    [[ -f /tmp/bench_result.txt ]] \
      && grep -qF "$expected" /tmp/bench_result.txt 2>/dev/null \
      && correct=1
  else
    correct=$completed  # no EXPECTED line → completion is sufficient
  fi

  # efficiency: 1.0 for ≤1 step, 0.0 at MAX_BENCH_STEPS steps
  local efficiency
  efficiency=$(LC_NUMERIC=C awk \
    -v s="$steps" -v m="$MAX_BENCH_STEPS" \
    'BEGIN {
      if      (s <= 1) printf "%.4f\n", 1.0
      else if (s >= m) printf "%.4f\n", 0.0
      else             printf "%.4f\n", (m - s) / (m - 1)
    }')

  # composite score
  local composite
  composite=$(LC_NUMERIC=C awk \
    -v c="$completed" -v r="$correct" -v e="$efficiency" \
    -v wc="$W_COMPLETED" -v wr="$W_CORRECT" -v we="$W_EFFICIENCY" \
    'BEGIN { printf "%.4f\n", c*wc + r*wr + e*we }')

  # Write result — note: "label" is a jq keyword so it must be quoted
  jq -n \
    --arg     bench       "$bench_name" \
    --arg     lbl         "$label" \
    --argjson cycle_n     "$cycle" \
    --argjson completed   "$completed" \
    --argjson correct     "$correct" \
    --argjson steps       "$steps" \
    --argjson duration    "$duration" \
    --argjson efficiency  "$efficiency" \
    --argjson composite   "$composite" \
    '{benchmark:$bench, label:$lbl, cycle:$cycle_n,
      completed:($completed==1), correct:($correct==1),
      steps:$steps, duration_s:$duration,
      efficiency:$efficiency, composite:$composite}' \
    > "$result_file"

  printf "    %-32s  completed=%s  correct=%s  steps=%-2s  → %s\n" \
    "$bench_name" "$completed" "$correct" "$steps" "$composite" >&2
}

# run_all_benchmarks BINARY LABEL CYCLE
# Prints aggregate composite score to stdout; everything else to stderr.
run_all_benchmarks() {
  local binary="$1" label="$2" cycle="$3"
  log "Benchmarking [$label]..."

  rm -f "$SCORES_DIR"/cycle_${cycle}_${label}_*.json \
        "$SCORES_DIR"/cycle_${cycle}_${label}_*.log

  local bench_count=0
  for bench_file in $(find "$BENCHMARKS_DIR" -name "*.md" | sort); do
    run_one_benchmark "$binary" "$label" "$cycle" "$bench_file"
    bench_count=$(( bench_count + 1 ))
  done

  [[ $bench_count -eq 0 ]] && { fail "No benchmark files in $BENCHMARKS_DIR"; exit 1; }

  local agg_file="$SCORES_DIR/cycle_${cycle}_${label}_aggregate.json"

  # Aggregate — "label" quoted to avoid jq keyword clash
  jq -s \
    --arg     lbl   "$label" \
    --argjson cyc   "$cycle" \
    '{"label":$lbl, cycle:$cyc, n:length,
      avg_composite:     (map(.composite)                           | add / length),
      completion_rate:   (map(if .completed then 1.0 else 0.0 end) | add / length),
      correctness_rate:  (map(if .correct   then 1.0 else 0.0 end) | add / length),
      avg_steps:         (map(.steps)                               | add / length),
      avg_efficiency:    (map(.efficiency)                          | add / length),
      benchmarks: .}' \
    "$SCORES_DIR"/cycle_${cycle}_${label}_0*.json \
    > "$agg_file"

  local avg; avg=$(jq '.avg_composite' "$agg_file")
  ok "[$label] composite=$avg  completion=$(jq '.completion_rate' "$agg_file")  correct=$(jq '.correctness_rate' "$agg_file")  steps=$(jq '.avg_steps' "$agg_file")" 
  echo "$avg"  # ← only stdout output; captured by caller's $()
}

# ═══════════════════════════════════════════════════════════════════════════
# PHASE 2+3 — REASON & IMPLEMENT
# ═══════════════════════════════════════════════════════════════════════════
phase_reason_and_implement() {
  local cycle="$1" next_revision="$2"
  local stable_agg="$SCORES_DIR/cycle_${cycle}_stable_aggregate.json"
  local reasoning_file="$REASONING_DIR/cycle_${cycle}.md"

  local prompt
  prompt=$(cat <<PROMPT
# Evolution Cycle ${cycle} — Improve Your Own Source Code

You are miniagent. Your job right now is to make ONE targeted improvement to
your own Go source code that will measurably improve your benchmark scores.

## Current benchmark scores (stable baseline)
\`\`\`json
$(cat "$stable_agg")
\`\`\`

## Benchmark definitions (what you are scored on)
$(for f in $(find "$BENCHMARKS_DIR" -name "*.md" | sort); do
    echo "### $(basename "$f")"; cat "$f"; echo; echo "---"
  done)

## Scoring formula
  composite  = completed×0.40 + correct×0.40 + efficiency×0.20
  efficiency = clamp(1 − steps÷${MAX_BENCH_STEPS}, 0, 1)
  A candidate must score > stable × ${IMPROVEMENT_THRESHOLD} to be accepted.

## Steps you MUST complete (do not skip any)

1. Read your source files (use cat — do NOT skip this step):
     /repo/cmd/miniagent/agent/system.go   ← system prompt  (HIGHEST IMPACT)
     /repo/cmd/miniagent/agent/tools.go    ← bash tool description + limits
     /repo/cmd/miniagent/main.go           ← CLI flag defaults

2. Identify ONE specific change that would improve the scores above.
   Think about cause and effect:
   - Low efficiency (many steps) → improve batching instructions in the system prompt
   - Low correctness → clarify what the agent must write to /tmp/bench_result.txt
   - Low completion → improve error recovery guidance

   Highest-impact levers (in order):
   a) System prompt in agent/system.go  — instructions to the LLM
   b) Bash tool description in agent/tools.go — how the tool is explained
   c) Default flag values in main.go    — timeout, max-steps, max-tokens

3. Implement the change by editing the file(s) directly with bash.

4. Verify the code compiles:
     cd /repo && go build ./cmd/miniagent/
   If it fails, FIX the error.
   If you truly cannot fix it, revert with:
     git -C /repo restore cmd/miniagent/agent/ cmd/miniagent/main.go
   and write NO_CHANGE as the first line of the reasoning file below.

5. Show the diff:
     git -C /repo diff cmd/miniagent/agent/ cmd/miniagent/main.go

6. Write your reasoning to: ${reasoning_file}
   Include what you changed, why, which benchmarks should improve, and the diff.
   If making no change: write "NO_CHANGE" as the very first line.

6. Update documentation (do this AFTER verifying go build succeeds):

   a) CHANGELOG.md at /repo/cmd/miniagent/CHANGELOG.md
      Add a new entry at the very top (after the header block, before any existing "---"):

        ## revision ${next_revision} — $(date +%Y-%m-%d)

        <one or two sentences: what you changed, which benchmarks you expect
        to improve, and by roughly how much>

        ---

   b) README.md at /repo/cmd/miniagent/README.md
      Update the "What it can do" bullet list ONLY if a user-visible
      capability was genuinely added or removed. Skip otherwise.

   c) AGENTS.md at /repo/cmd/miniagent/AGENTS.md
      Update ONLY if the architecture, scoring rules, or agent constraints
      actually changed. Skip otherwise.

Rules:
- ONE focused change only
- Code MUST compile after your changes
- Do NOT add new external dependencies
- Do NOT modify benchmark files, evolve/ files, or the agent loop in agent/agent.go
PROMPT
)

  log "Running reason & implement phase (stable agent)..."

  if [[ "$DRY_RUN" == true ]]; then
    warn "[DRY RUN] Skipping LLM reasoning — writing NO_CHANGE placeholder"
    echo "NO_CHANGE (dry run)" > "$reasoning_file"
    return 1
  fi

  set +e
  "$STABLE_BIN" \
    --workspace "$REPO_ROOT" \
    --max-steps 30 \
    --timeout   60s \
    "$prompt"
  local exit_code=$?
  set -e

  [[ $exit_code -ne 0 ]] && warn "Reasoning agent exited $exit_code"

  if [[ -f "$reasoning_file" ]] && grep -q "^NO_CHANGE" "$reasoning_file"; then
    warn "Agent decided no improvement is possible this cycle"
    return 1
  fi

  if [[ ! -f "$reasoning_file" ]]; then
    warn "No reasoning file written — treating as NO_CHANGE"
    return 1
  fi

  ok "Reasoning and implementation complete"
  return 0
}

# ═══════════════════════════════════════════════════════════════════════════
# PHASE 4 — BUILD CANDIDATE
# ═══════════════════════════════════════════════════════════════════════════
phase_build_candidate() {
  local cycle="$1"
  log "Building candidate binary..."

  if [[ "$DRY_RUN" == true ]]; then
    warn "[DRY RUN] Copying stable as fake candidate"
    cp "$STABLE_BIN" "$CANDIDATE_BIN"
    chmod +x "$CANDIDATE_BIN"
    return 0
  fi

  set +e
  (
    cd "$REPO_ROOT"
    export GOCACHE="${GOCACHE:-/repo/.go-cache/build}"
    export GOMODCACHE="${GOMODCACHE:-/repo/.go-cache/mod}"
    export GOTOOLCHAIN=local
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o "$CANDIDATE_BIN" \
      ./cmd/miniagent/
  )
  local exit_code=$?
  set -e

  if [[ $exit_code -ne 0 ]]; then
    fail "Candidate build failed — reverting source changes"
    git -C "$REPO_ROOT" restore cmd/miniagent/agent/ cmd/miniagent/main.go 2>/dev/null || true
    return 1
  fi

  chmod +x "$CANDIDATE_BIN"
  ok "Candidate built: $CANDIDATE_BIN ($(du -sh "$CANDIDATE_BIN" | cut -f1))"
  return 0
}

# ═══════════════════════════════════════════════════════════════════════════
# PHASE 6 — JUDGE  (pure arithmetic, no LLM)
# ═══════════════════════════════════════════════════════════════════════════
phase_judge() {
  local cycle="$1"
  local ss; ss=$(jq '.avg_composite' "$SCORES_DIR/cycle_${cycle}_stable_aggregate.json")
  local cs; cs=$(jq '.avg_composite' "$SCORES_DIR/cycle_${cycle}_candidate_aggregate.json")

  log "Judge: stable=$ss  candidate=$cs  threshold=×${IMPROVEMENT_THRESHOLD}"

  local keep
  keep=$(LC_NUMERIC=C awk -v cs="$cs" -v ss="$ss" -v t="$IMPROVEMENT_THRESHOLD" \
    'BEGIN { print (cs > ss * t) ? "1" : "0" }')

  if [[ "$keep" == "1" ]]; then
    ok "KEEP — candidate ($cs) beats stable ($ss) × $IMPROVEMENT_THRESHOLD"
    return 0
  else
    warn "REVERT — candidate ($cs) did not beat stable ($ss) × $IMPROVEMENT_THRESHOLD"
    return 1
  fi
}

# ═══════════════════════════════════════════════════════════════════════════
# PHASE 7a — COMMIT & PROMOTE
# ═══════════════════════════════════════════════════════════════════════════
phase_commit_and_promote() {
  local cycle="$1" next_revision="$2"

  local summary
  summary=$(grep -v '^#' "$REASONING_DIR/cycle_${cycle}.md" 2>/dev/null \
    | grep -m1 '\S' | cut -c1-72 || echo "self-improvement")

  log "Committing improvements..."
  git -C "$REPO_ROOT" add \
    cmd/miniagent/agent/ \
    cmd/miniagent/main.go \
    cmd/miniagent/CHANGELOG.md \
    cmd/miniagent/README.md \
    cmd/miniagent/AGENTS.md
  git -C "$REPO_ROOT" commit \
    -m "evolve(cycle${cycle}): ${summary}" \
    -m "See cmd/miniagent/evolve/reasoning/cycle_${cycle}.md"

  cp "$CANDIDATE_BIN" "$STABLE_BIN"
  chmod +x "$STABLE_BIN"
  local new_commit; new_commit=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
  echo "$new_commit" > "$BIN_DIR/stable_commit"
  # Persist incremented revision
  echo "$next_revision" > "$REVISION_FILE"
  CURRENT_REVISION=$next_revision
  ok "Promoted candidate to stable (HEAD: $new_commit, revision: $next_revision)"
}

# ═══════════════════════════════════════════════════════════════════════════
# PHASE 7b — REVERT
# ═══════════════════════════════════════════════════════════════════════════
phase_revert() {
  log "Reverting source changes..."
  git -C "$REPO_ROOT" restore \
    cmd/miniagent/agent/ cmd/miniagent/main.go 2>/dev/null || true
  rm -f "$CANDIDATE_BIN"
  warn "Changes reverted — stable binary unchanged"
}

# ═══════════════════════════════════════════════════════════════════════════
# MAIN LOOP
# ═══════════════════════════════════════════════════════════════════════════
echo "" >&2
echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════╗${RESET}" >&2
echo -e "${BOLD}${CYAN}║  🧬  miniagent self-improvement loop                         ║${RESET}" >&2
echo -e "${BOLD}${CYAN}║  benchmark → reason → implement → build → benchmark → judge  ║${RESET}" >&2
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════╝${RESET}" >&2
[[ "$DRY_RUN" == true ]] && warn "DRY RUN MODE — no LLM calls or file edits"
echo "" >&2

# Sanity checks
git -C "$REPO_ROOT" rev-parse HEAD &>/dev/null \
  || { fail "Not a git repo: $REPO_ROOT"; exit 1; }
[[ $(find "$BENCHMARKS_DIR" -name "*.md" 2>/dev/null | wc -l) -gt 0 ]] \
  || { fail "No benchmarks in $BENCHMARKS_DIR"; exit 1; }

phase_setup

cycle=0
total_improvements=0
total_reverts=0

while true; do
  cycle=$(( cycle + 1 ))
  header "══════════════════════════════════════════════════════════════"
  header "  Cycle ${cycle}  —  $(date '+%Y-%m-%d %H:%M:%S')"
  header "══════════════════════════════════════════════════════════════"

  # [1] Baseline
  header "  [1/6] Baseline benchmarks (stable)"
  stable_score=$(run_all_benchmarks "$STABLE_BIN" "stable" "$cycle")

  # [2+3] Reason & implement
  header "  [2/6] Reason & implement"
  next_revision=$(( CURRENT_REVISION + 1 ))

  if ! phase_reason_and_implement "$cycle" "$next_revision"; then
    warn "No change this cycle — skipping build/benchmark"
    [[ "$MAX_CYCLES" -gt 0 && "$cycle" -ge "$MAX_CYCLES" ]] && break
    continue
  fi

  # [4] Build candidate
  header "  [3/6] Build candidate"
  if ! phase_build_candidate "$cycle"; then
    fail "Build failed — skipping benchmark"
    total_reverts=$(( total_reverts + 1 ))
    [[ "$MAX_CYCLES" -gt 0 && "$cycle" -ge "$MAX_CYCLES" ]] && break
    continue
  fi

  # [5] Benchmark candidate
  header "  [4/6] Benchmark candidate"
  candidate_score=$(run_all_benchmarks "$CANDIDATE_BIN" "candidate" "$cycle")

  # [6] Judge
  header "  [5/6] Judge"
  if phase_judge "$cycle"; then
    header "  [6/6] Commit & promote"
    phase_commit_and_promote "$cycle" "$next_revision"
    total_improvements=$(( total_improvements + 1 ))
  else
    header "  [6/6] Revert"
    phase_revert
    total_reverts=$(( total_reverts + 1 ))
  fi

  # Append cycle summary to the JSONL history log
  jq -n \
    --argjson cycle              "$cycle" \
    --argjson stable_score       "$stable_score" \
    --argjson candidate_score    "$candidate_score" \
    --argjson total_improvements "$total_improvements" \
    --argjson total_reverts      "$total_reverts" \
    '{cycle:$cycle, stable_score:$stable_score,
      candidate_score:$candidate_score,
      total_improvements:$total_improvements,
      total_reverts:$total_reverts}' \
    >> "$EVOLVE_DIR/cycles.jsonl"

  [[ "$MAX_CYCLES" -gt 0 && "$cycle" -ge "$MAX_CYCLES" ]] && break
done

echo "" >&2
echo -e "${BOLD}${GREEN}═══════════════════════════════════════════════${RESET}" >&2
echo -e "${BOLD}${GREEN}  Evolution complete${RESET}" >&2
echo "  Cycles run:         $cycle" >&2
echo "  Improvements kept:  $total_improvements" >&2
echo "  Reverts:            $total_reverts" >&2
echo "  Stable commit:      $(cat "$BIN_DIR/stable_commit" 2>/dev/null || echo 'unknown')" >&2
echo -e "${BOLD}${GREEN}═══════════════════════════════════════════════${RESET}" >&2
