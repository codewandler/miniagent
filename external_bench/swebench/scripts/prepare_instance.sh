#!/usr/bin/env bash
set -euo pipefail

instance_json="${1:?usage: prepare_instance.sh <instance.json> <instance_dir>}"
instance_dir="${2:?usage: prepare_instance.sh <instance.json> <instance_dir>}"
workspace="$instance_dir/workspace"
prep_log="$instance_dir/prep.log"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
rm -rf "$workspace"
mkdir -p "$workspace"
setup_strategy=$(jq -r '.setup_strategy // ""' "$instance_json")
repo=$(jq -r '.repo' "$instance_json")
base_commit=$(jq -r '.base_commit' "$instance_json")
python_version=$(jq -r '.python_version // empty' "$instance_json")
{
  echo "instance_json=$instance_json"
  echo "setup_strategy=$setup_strategy"
  echo "workspace=$workspace"
  if [[ "$setup_strategy" == "go-test" || "$setup_strategy" == "swebench-go" ]]; then
    fixture_rel=$(jq -r '.fixture_path' "$instance_json")
    fixture_abs="$repo_root/$fixture_rel"
    echo "fixture=$fixture_abs"
    cp -R "$fixture_abs"/. "$workspace"/
    if [ -d "$workspace/.git" ]; then rm -rf "$workspace/.git"; fi
    git -C "$workspace" init -q
    git -C "$workspace" config user.name swebench
    git -C "$workspace" config user.email swebench@example.local
    git -C "$workspace" add .
    git -C "$workspace" commit -q -m 'baseline fixture'
    if jq -e '.install_command? != null and .install_command != ""' "$instance_json" >/dev/null 2>&1; then
      install_cmd=$(jq -r '.install_command' "$instance_json")
      PATH=/usr/lib/go-1.19/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin bash -lc "cd '$workspace' && GOFLAGS=-mod=mod $install_cmd"
    fi
  elif [[ "$setup_strategy" == "swebench-python-django" ]]; then
    cache_dir="$repo_root/external_bench/swebench/cache/repos/$(printf '%s' "$repo" | tr '/' '__')"
    if [[ ! -d "$cache_dir/.git" ]]; then
      git clone --filter=blob:none "https://github.com/$repo" "$cache_dir"
    fi
    git -C "$cache_dir" fetch --all --tags --prune
    cp -R "$cache_dir"/. "$workspace"/
    rm -rf "$workspace/.git"
    git -C "$workspace" init -q
    git -C "$workspace" config user.name swebench
    git -C "$workspace" config user.email swebench@example.local
    git -C "$workspace" add .
    git -C "$workspace" commit -q -m 'pre-reset import'
    git -C "$workspace" remote add origin "https://github.com/$repo"
    git -C "$workspace" fetch --depth=1 origin "$base_commit"
    git -C "$workspace" reset --hard "$base_commit"
    python3 -m venv "$workspace/.venv"
    . "$workspace/.venv/bin/activate"
    python -m pip install --upgrade pip setuptools wheel
    pip install -e "$workspace"
    if jq -e '.pip_packages? | length > 0' "$instance_json" >/dev/null 2>&1; then
      pip install $(jq -r '.pip_packages[]' "$instance_json" | tr '
' ' ')
    fi
  else
    echo "unsupported setup_strategy=$setup_strategy" >&2
    exit 2
  fi
  echo "$workspace"
} > "$prep_log" 2>&1
printf '%s\n' "$workspace"
