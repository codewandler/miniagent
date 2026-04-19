#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SWE_DIR="$ROOT_DIR/external_bench/swebench"
INSTANCE_LIST="${1:-$SWE_DIR/instances/curated_lite.txt}"
RUN_LABEL="${SWE_RUN_LABEL:-manual}"
RUN_ID="$(date +%Y%m%d_%H%M%S)_${RUN_LABEL}_$(printf '%s' "${SWE_DRIVER:-oracle}" | tr -c 'A-Za-z0-9_-' '_')"
RUN_DIR="$SWE_DIR/results/$RUN_ID"
mkdir -p "$RUN_DIR"
while IFS= read -r line; do
  [[ -z "$line" || "$line" =~ ^# ]] && continue
  instance_path="$ROOT_DIR/$line"
  instance_id="$(jq -r '.instance_id' "$instance_path")"
  instance_dir="$RUN_DIR/$instance_id"
  mkdir -p "$instance_dir"
  cp "$instance_path" "$instance_dir/instance.json"
  workspace="$($SWE_DIR/scripts/prepare_instance.sh "$instance_path" "$instance_dir")"
  meta_json="$($SWE_DIR/scripts/run_agent_on_instance.sh "$instance_path" "$workspace" "$instance_dir")"
  eval_status="$($SWE_DIR/scripts/evaluate_instance.sh "$instance_path" "$workspace" "$instance_dir")"
  jq -n \
    --slurpfile inst "$instance_path" \
    --slurpfile meta "$meta_json" \
    --arg run_label "$RUN_LABEL" \
    --arg workspace "$workspace" \
    --arg diff_path "$instance_dir/diff.patch" \
    --arg log_path "$instance_dir/run.log" \
    --arg eval_log_path "$instance_dir/eval.log" \
    --argjson eval_status "$eval_status" \
    '{instance_id:$inst[0].instance_id,run_label:$run_label,cycle:0,completed:($meta[0].exit_code==0),resolved:($eval_status==0),steps:$meta[0].steps,duration_s:$meta[0].duration_s,files_changed:$meta[0].files_changed,exit_code:$meta[0].exit_code,driver:$meta[0].driver,workspace:$workspace,diff_path:$diff_path,log_path:$log_path,eval_log_path:$eval_log_path}' > "$instance_dir/result.json"
  printf 'instance=%s completed=%s resolved=%s steps=%s duration_s=%s\n' \
    "$instance_id" "$(jq -r '.completed' "$instance_dir/result.json")" "$(jq -r '.resolved' "$instance_dir/result.json")" "$(jq -r '.steps' "$instance_dir/result.json")" "$(jq -r '.duration_s' "$instance_dir/result.json")"
done < "$INSTANCE_LIST"
"$SWE_DIR/scripts/aggregate_results.sh" "$RUN_DIR" > "$RUN_DIR/aggregate.json"
printf 'run_dir=%s\naggregate=%s\n' "$RUN_DIR" "$RUN_DIR/aggregate.json"
