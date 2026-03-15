#!/usr/bin/env bash

set -euo pipefail

input_dir="${INPUT_DIR:-/input}"
output_dir="${OUTPUT_DIR:-/output}"
output_name="${OUTPUT_NAME:-}"
mode="${TIMELAPSE_MODE:-once}"
interval_seconds="${TIMELAPSE_INTERVAL_SECONDS:-3600}"
from_value="${TIMELAPSE_FROM:-}"
to_value="${TIMELAPSE_TO:-}"

mkdir -p "$output_dir"

build_output_path() {
  if [[ -n "$output_name" ]]; then
    printf '%s/%s\n' "$output_dir" "$output_name"
    return
  fi
  printf '%s/timelapse-%s.mp4\n' "$output_dir" "$(date +%Y%m%d-%H%M%S)"
}

run_timelapse() {
  local output_path
  output_path="$(build_output_path)"
  local -a cmd=(/usr/local/bin/make-timelapse.sh)

  if [[ -n "$from_value" ]]; then
    cmd+=(--from "$from_value")
  fi
  if [[ -n "$to_value" ]]; then
    cmd+=(--to "$to_value")
  fi

  cmd+=("$input_dir" "$output_path")

  printf 'Running timelapse build: %s\n' "$output_path"
  exec_cmd=(env \
    INPUT_FPS="${INPUT_FPS:-8}" \
    OUTPUT_FPS="${OUTPUT_FPS:-30}" \
    CRF="${CRF:-23}" \
    MIN_BRIGHTNESS="${MIN_BRIGHTNESS:-0}")
  "${exec_cmd[@]}" "${cmd[@]}"
}

case "$mode" in
  once)
    run_timelapse
    ;;
  loop)
    while true; do
      run_timelapse
      sleep "$interval_seconds"
    done
    ;;
  *)
    printf 'Unsupported TIMELAPSE_MODE: %s\n' "$mode" >&2
    exit 1
    ;;
esac
