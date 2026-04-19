#!/usr/bin/env bash
set -euo pipefail

instance_json="${1:?usage: evaluate_instance.sh <instance.json> <workspace> <instance_dir>}"
workspace="${2:?usage: evaluate_instance.sh <instance.json> <workspace> <instance_dir>}"
instance_dir="${3:?usage: evaluate_instance.sh <instance.json> <workspace> <instance_dir>}"
eval_log="$instance_dir/eval.log"
test_cmd=$(jq -r '.validate_command' "$instance_json")
setup_strategy=$(jq -r '.setup_strategy // ""' "$instance_json")
set +e
if [[ "$setup_strategy" == "swebench-python-django" ]]; then
  test_patch_file="$instance_dir/test.patch"
  jq -r '.test_patch' "$instance_json" > "$test_patch_file"
  PATH=/usr/lib/go-1.19/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin bash -lc "cd '$workspace' && git apply '$test_patch_file' && . .venv/bin/activate && $test_cmd" > "$eval_log" 2>&1
else
  PATH=/usr/lib/go-1.19/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin bash -lc "cd '$workspace' && $test_cmd" > "$eval_log" 2>&1
fi
status=$?
set -e
printf '%s
' "$status"
