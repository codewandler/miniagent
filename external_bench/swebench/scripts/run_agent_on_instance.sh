#!/usr/bin/env bash
set -euo pipefail

instance_json="${1:?usage: run_agent_on_instance.sh <instance.json> <workspace> <instance_dir>}"
workspace="${2:?usage: run_agent_on_instance.sh <instance.json> <workspace> <instance_dir>}"
instance_dir="${3:?usage: run_agent_on_instance.sh <instance.json> <workspace> <instance_dir>}"
driver="${SWE_DRIVER:-oracle}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
appendix="$repo_root/external_bench/swebench/prompts/system_appendix.txt"
run_log="$instance_dir/run.log"
meta_json="$instance_dir/run_meta.json"
diff_path="$instance_dir/diff.patch"
changed_files="$instance_dir/changed_files.txt"
issue_text=$(jq -r '.problem_statement // .issue_text' "$instance_json")
hints_text=$(jq -r '.hints_text // empty' "$instance_json")
timeout_sec=$(jq -r '.timeout_sec // 300' "$instance_json")
start=$(date +%s)
exit_code=0
steps=0
setup_strategy=$(jq -r '.setup_strategy // ""' "$instance_json")
if [ "$driver" = "oracle" ]; then
  if [[ "$setup_strategy" == "go-test" || "$setup_strategy" == "swebench-go" ]]; then
    gold_patch_rel=$(jq -r '.gold_patch // empty' "$instance_json")
    gold_patch="$workspace/$gold_patch_rel"
    {
      echo "driver=oracle"
      echo "applying $gold_patch"
      git -C "$workspace" apply "$gold_patch"
    } > "$run_log" 2>&1 || exit_code=$?
    steps=1
  else
    patch_file="$instance_dir/oracle.patch"
    jq -r '.patch' "$instance_json" > "$patch_file"
    {
      echo "driver=oracle"
      echo "applying dataset patch"
      git -C "$workspace" apply "$patch_file"
    } > "$run_log" 2>&1 || exit_code=$?
    steps=1
  fi
elif [ "$driver" = "agent" ]; then
  miniagent_bin="${MINIAGENT_BIN:-$repo_root/.miniagent-bin}"
  prompt_file="$instance_dir/prompt.txt"
  {
    cat "$appendix"
    printf '\nIssue:\n%s\n' "$issue_text"
    if [ -n "$hints_text" ]; then printf '\nHints:\n%s\n' "$hints_text"; fi
    printf '\nInstructions:\nFix the bug in this repository, run the relevant tests, and stop only after the repository is in a passing state if possible.\n'
  } > "$prompt_file"
  set +e
  if [[ "$setup_strategy" == "swebench-python-django" ]]; then
    timeout "$timeout_sec" bash -lc "cd '$workspace' && . .venv/bin/activate && '$miniagent_bin' --workspace '$workspace' --max-steps '${SWE_MAX_STEPS:-40}' --timeout '${SWE_TOTAL_TIMEOUT:-20m}' --tool-timeout '${SWE_TOOL_TIMEOUT:-120s}' \"$(cat "$prompt_file")\"" > "$run_log" 2>&1
  else
    timeout "$timeout_sec" "$miniagent_bin" --workspace "$workspace" --max-steps "${SWE_MAX_STEPS:-40}" --timeout "${SWE_TOTAL_TIMEOUT:-10m}" --tool-timeout "${SWE_TOOL_TIMEOUT:-60s}" "$(cat "$prompt_file")" > "$run_log" 2>&1
  fi
  exit_code=$?
  set -e
  steps=$(awk '/🔧/{c++} END{print c+0}' "$run_log")
else
  echo "unsupported SWE_DRIVER=$driver" > "$run_log"
  exit_code=2
fi
end=$(date +%s)
duration=$(( end - start ))
git -C "$workspace" diff > "$diff_path" || true
git -C "$workspace" diff --name-only > "$changed_files" || true
files_changed=$(awk 'NF{c++} END{print c+0}' "$changed_files")
jq -n --arg driver "$driver" --argjson exit_code "$exit_code" --argjson steps "$steps" --argjson duration_s "$duration" --argjson files_changed "$files_changed" '{driver:$driver, exit_code:$exit_code, steps:$steps, duration_s:$duration_s, files_changed:$files_changed}' > "$meta_json"
printf '%s\n' "$meta_json"
