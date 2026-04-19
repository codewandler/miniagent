#!/usr/bin/env bash
set -euo pipefail

run_dir="${1:?usage: aggregate_results.sh <run_dir>}"
jq -s '{n:length, resolved_rate:(map(if .resolved then 1 else 0 end)|add/length), completion_rate:(map(if .completed then 1 else 0 end)|add/length), avg_steps:(map(.steps)|add/length), avg_duration_s:(map(.duration_s)|add/length), instances: map(.instance_id)}' "$run_dir"/*/result.json
