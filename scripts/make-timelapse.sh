#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: make-timelapse.sh [--from DATETIME] [--to DATETIME] [input_dir] [output_file]

Build an MP4 timelapse from images stored in date-based subdirectories.

Arguments:
  input_dir    Root directory containing dated folders
               Default: /home/andy/Pictures/timelapse
  output_file  Output MP4 path
               Default: ./timelapse-YYYYmmdd-HHMMSS.mp4

Options:
  --from DATETIME  Include frames at or after this time
  --to DATETIME    Include frames at or before this time
                   Accepted examples: 2026-03-15, 2026-03-15 08:00:00,
                   20260315 080000, 2026-03-15T08:00:00

Environment:
  INPUT_FPS    Input image rate used to pace images in the timelapse (default: 8)
  OUTPUT_FPS   Output video fps (default: 30)
  CRF          x264 quality factor, lower is better quality (default: 23)
  MIN_BRIGHTNESS  Skip frames darker than this 0-1 value when ImageMagick is
                  available (default: 0, disabled)

Examples:
  scripts/make-timelapse.sh
  scripts/make-timelapse.sh /home/andy/Pictures/timelapse greenhouse.mp4
  scripts/make-timelapse.sh --from "2026-03-15 08:00:00" --to "2026-03-15 18:00:00"
  INPUT_FPS=12 OUTPUT_FPS=24 scripts/make-timelapse.sh
EOF
}

from_arg=''
to_arg=''
positionals=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --from)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--from requires a value' >&2
        exit 1
      fi
      from_arg="$2"
      shift 2
      ;;
    --to)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--to requires a value' >&2
        exit 1
      fi
      to_arg="$2"
      shift 2
      ;;
    --)
      shift
      while [[ $# -gt 0 ]]; do
        positionals+=("$1")
        shift
      done
      ;;
    -*)
      printf 'Unknown option: %s\n' "$1" >&2
      exit 1
      ;;
    *)
      positionals+=("$1")
      shift
      ;;
  esac
done

if ! command -v ffmpeg >/dev/null 2>&1; then
  printf 'ffmpeg is required but was not found in PATH\n' >&2
  exit 1
fi

input_dir="${positionals[0]:-/home/andy/Pictures/timelapse}"
output_file="${positionals[1]:-timelapse-$(date +%Y%m%d-%H%M%S).mp4}"
input_fps="${INPUT_FPS:-8}"
output_fps="${OUTPUT_FPS:-30}"
crf="${CRF:-23}"
min_brightness="${MIN_BRIGHTNESS:-0}"
skipped_frames=0
skipped_range=0

parse_datetime() {
  local value="$1"
  date -d "$value" +%s 2>/dev/null
}

extract_image_epoch() {
  local image="$1"
  local base
  base="$(basename "$image")"

  if [[ "$base" =~ ([0-9]{8})_([0-9]{6}) ]]; then
    date -d "${BASH_REMATCH[1]} ${BASH_REMATCH[2]}" +%s 2>/dev/null
    return
  fi

  stat -c %Y "$image" 2>/dev/null
}

from_epoch=''
to_epoch=''

if [[ -n "$from_arg" ]]; then
  from_epoch="$(parse_datetime "$from_arg")" || {
    printf 'Invalid --from value: %s\n' "$from_arg" >&2
    exit 1
  }
fi

if [[ -n "$to_arg" ]]; then
  to_epoch="$(parse_datetime "$to_arg")" || {
    printf 'Invalid --to value: %s\n' "$to_arg" >&2
    exit 1
  }
fi

if [[ -n "$from_epoch" && -n "$to_epoch" && "$from_epoch" -gt "$to_epoch" ]]; then
  printf '%s\n' '--from must be earlier than or equal to --to' >&2
  exit 1
fi

brightness_tool=''
if command -v magick >/dev/null 2>&1; then
  brightness_tool='magick'
elif command -v convert >/dev/null 2>&1; then
  brightness_tool='convert'
fi

if [[ ! -d "$input_dir" ]]; then
  printf 'Input directory not found: %s\n' "$input_dir" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
list_file="$tmp_dir/frames.txt"
trap 'rm -rf "$tmp_dir"' EXIT

mapfile -d '' image_files < <(
  find "$input_dir" -type f \( \
    -iname '*.jpg' -o \
    -iname '*.jpeg' -o \
    -iname '*.png' -o \
    -iname '*.webp' \
  \) -print0 | sort -z
)

if [[ ${#image_files[@]} -eq 0 ]]; then
  printf 'No images found under: %s\n' "$input_dir" >&2
  exit 1
fi

frame_duration="$(awk -v fps="$input_fps" 'BEGIN { printf "%.6f", 1 / fps }')"

included_count=0
last_included_image=''

for image in "${image_files[@]}"; do
  if [[ -n "$from_epoch" || -n "$to_epoch" ]]; then
    image_epoch="$(extract_image_epoch "$image")"
    if [[ -z "$image_epoch" ]]; then
      printf 'Warning: could not determine timestamp for %s\n' "$image" >&2
      skipped_range=$((skipped_range + 1))
      continue
    fi
    if [[ -n "$from_epoch" && "$image_epoch" -lt "$from_epoch" ]]; then
      skipped_range=$((skipped_range + 1))
      continue
    fi
    if [[ -n "$to_epoch" && "$image_epoch" -gt "$to_epoch" ]]; then
      skipped_range=$((skipped_range + 1))
      continue
    fi
  fi

  if [[ "$min_brightness" != "0" && -n "$brightness_tool" ]]; then
    brightness="$($brightness_tool "$image" -colorspace Gray -format '%[fx:mean]' info: 2>/dev/null || true)"
    if [[ -z "$brightness" ]]; then
      printf 'Warning: could not measure brightness for %s\n' "$image" >&2
    elif ! awk -v b="$brightness" -v min="$min_brightness" 'BEGIN { exit !(b >= min) }'; then
      skipped_frames=$((skipped_frames + 1))
      printf 'Skipping dark frame (%s): %s\n' "$brightness" "$image"
      continue
    fi
  fi

  printf "file '%s'\n" "$image" >>"$list_file"
  printf 'duration %s\n' "$frame_duration" >>"$list_file"
  last_included_image="$image"
  included_count=$((included_count + 1))
done

if [[ "$min_brightness" != "0" && -z "$brightness_tool" ]]; then
  printf 'Warning: MIN_BRIGHTNESS is set but ImageMagick is not installed; skipping dark-frame filtering\n' >&2
fi

if [[ "$included_count" -eq 0 ]]; then
  printf 'No usable images found after filtering\n' >&2
  exit 1
fi

# concat demuxer requires the last file to be listed again when durations are used
printf "file '%s'\n" "$last_included_image" >>"$list_file"

printf 'Building timelapse from %d images' "$included_count"
if [[ "$skipped_range" -gt 0 ]]; then
  printf ' (%d outside requested range)' "$skipped_range"
fi
if [[ "$skipped_frames" -gt 0 ]]; then
  printf ' (%d dark frames skipped)' "$skipped_frames"
fi
printf '...\n'

ffmpeg -y \
  -f concat \
  -safe 0 \
  -i "$list_file" \
  -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p" \
  -r "$output_fps" \
  -c:v libx264 \
  -crf "$crf" \
  -movflags +faststart \
  "$output_file"

printf 'Created %s\n' "$output_file"
