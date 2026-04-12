#!/usr/bin/env bash
# ╔══════════════════════════════════════════════════════════════════════════╗
# ║  HerbHub365 — Video Overlay Render Script                               ║
# ║  Renders HTML templates → transparent PNG → composites onto video        ║
# ╚══════════════════════════════════════════════════════════════════════════╝
#
# PREREQUISITES
#   macOS:  brew install --cask google-chrome  (or chromium)
#           brew install ffmpeg
#   Linux:  apt install chromium-browser ffmpeg  (or snap install chromium)
#
# QUICK START (static overlay, default sample data)
#   cd Resources/video-overlays
#   ./render-overlay.sh --variant corner --input /path/to/input.mp4
#
# QUICK START (inject live data from Prometheus / environment vars)
#   export CHILLI_TEMP="31.2°"  CHILLI_SOIL="14%"  CHILLI_STATUS="alert"
#   export BASIL_TEMP="22.4°"   BASIL_SOIL="44%"   BASIL_STATUS="monitor"
#   # ... etc.
#   ./render-overlay.sh --variant corner --input clip.mp4 --output out.mp4 --live
#
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Paths ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CORNER_HTML="${SCRIPT_DIR}/overlay-card--corner.html"
LOWER_HTML="${SCRIPT_DIR}/overlay-card--lower-third.html"
SIDEBAR_HTML="${SCRIPT_DIR}/overlay-card--sidebar.html"

CORNER_PNG="${SCRIPT_DIR}/overlay-corner.png"
LOWER_PNG="${SCRIPT_DIR}/overlay-lower-third.png"
SIDEBAR_PNG="${SCRIPT_DIR}/overlay-sidebar.png"

# ── Defaults ─────────────────────────────────────────────────────────────────
VARIANT="corner"          # corner | lower-third | sidebar
INPUT_VIDEO=""
OUTPUT_VIDEO=""
LIVE_DATA=false
POSITION_X=20             # ffmpeg overlay X offset (corner variants)
POSITION_Y=20             # ffmpeg overlay Y offset (corner variants)
CORNER="tl"               # tl | tr | bl | br — corner placement shorthand
QUALITY=18                # ffmpeg CRF (18=high quality, 23=default, 28=smaller)

# ── Chromium binary detection ─────────────────────────────────────────────────
find_chromium() {
  # macOS: prefer Google Chrome, fall back to Chromium
  if [[ "$(uname)" == "Darwin" ]]; then
    local mac_chrome="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
    local mac_chromium="/Applications/Chromium.app/Contents/MacOS/Chromium"
    if [[ -x "${mac_chrome}" ]];   then echo "${mac_chrome}";   return; fi
    if [[ -x "${mac_chromium}" ]]; then echo "${mac_chromium}"; return; fi
  fi
  # Linux / PATH fallback
  for cmd in chromium chromium-browser google-chrome google-chrome-stable; do
    if command -v "${cmd}" &>/dev/null; then echo "${cmd}"; return; fi
  done
  echo ""
}

CHROMIUM="$(find_chromium)"
if [[ -z "${CHROMIUM}" ]]; then
  echo "ERROR: No Chromium/Chrome binary found."
  echo "  macOS:  brew install --cask google-chrome"
  echo "  Linux:  apt install chromium-browser"
  exit 1
fi

# ── CLI parsing ───────────────────────────────────────────────────────────────
usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

  --variant <corner|lower-third|sidebar>   Overlay layout (default: corner)
  --input   <file>                         Source video path (required for composite)
  --output  <file>                         Output video path (default: output-<variant>.mp4)
  --corner  <tl|tr|bl|br>                  Corner placement for corner variant (default: tl)
  --live                                   Inject live data from env vars (see below)
  --render-only                            Render PNG(s) only, skip ffmpeg
  --quality <crf>                          ffmpeg CRF quality 0-51 (default: 18)
  --help                                   Show this help

Env vars for --live data injection (all optional, fall back to template defaults):
  TIMESTAMP            e.g. "14:23:07"
  CHILLI_TEMP          e.g. "31.2°"
  CHILLI_SOIL          e.g. "14%"
  CHILLI_SOIL_PCT      e.g. "14"        (integer, for sidebar moisture bar)
  CHILLI_HUMIDITY      e.g. "48%"
  CHILLI_STATUS        healthy|monitor|alert
  BASIL_TEMP, BASIL_SOIL, BASIL_SOIL_PCT, BASIL_HUMIDITY, BASIL_STATUS
  OREGANO_TEMP, OREGANO_SOIL, OREGANO_SOIL_PCT, OREGANO_HUMIDITY, OREGANO_STATUS
  MINT_TEMP, MINT_SOIL, MINT_SOIL_PCT, MINT_HUMIDITY, MINT_STATUS
  AVG_TEMP             e.g. "25.0°C"
  AVG_SOIL             e.g. "42%"
  HUB_UPTIME           e.g. "14d 6h"
  ALERT_COUNT          e.g. "2"
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --variant)     VARIANT="$2";       shift 2 ;;
    --input)       INPUT_VIDEO="$2";   shift 2 ;;
    --output)      OUTPUT_VIDEO="$2";  shift 2 ;;
    --corner)      CORNER="$2";        shift 2 ;;
    --live)        LIVE_DATA=true;     shift   ;;
    --render-only) RENDER_ONLY=true;   shift   ;;
    --quality)     QUALITY="$2";       shift 2 ;;
    --help|-h)     usage ;;
    *) echo "Unknown option: $1"; usage ;;
  esac
done

RENDER_ONLY="${RENDER_ONLY:-false}"

# ── Build URL query string from env vars ──────────────────────────────────────
build_query_string() {
  local params=""
  append() {
    local key="$1" envvar="$2"
    local val="${!envvar:-}"
    [[ -n "${val}" ]] && params+="${params:+&}${key}=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "${val}")"
  }

  append "timestamp"       "TIMESTAMP"
  append "chilli_temp"     "CHILLI_TEMP"
  append "chilli_soil"     "CHILLI_SOIL"
  append "chilli_soil_pct" "CHILLI_SOIL_PCT"
  append "chilli_humidity" "CHILLI_HUMIDITY"
  append "chilli_status"   "CHILLI_STATUS"
  append "basil_temp"      "BASIL_TEMP"
  append "basil_soil"      "BASIL_SOIL"
  append "basil_soil_pct"  "BASIL_SOIL_PCT"
  append "basil_humidity"  "BASIL_HUMIDITY"
  append "basil_status"    "BASIL_STATUS"
  append "oregano_temp"    "OREGANO_TEMP"
  append "oregano_soil"    "OREGANO_SOIL"
  append "oregano_soil_pct" "OREGANO_SOIL_PCT"
  append "oregano_humidity" "OREGANO_HUMIDITY"
  append "oregano_status"  "OREGANO_STATUS"
  append "mint_temp"       "MINT_TEMP"
  append "mint_soil"       "MINT_SOIL"
  append "mint_soil_pct"   "MINT_SOIL_PCT"
  append "mint_humidity"   "MINT_HUMIDITY"
  append "mint_status"     "MINT_STATUS"
  append "avg_temp"        "AVG_TEMP"
  append "avg_soil"        "AVG_SOIL"
  append "hub_uptime"      "HUB_UPTIME"
  append "alert_count"     "ALERT_COUNT"

  echo "${params}"
}

# ── Render HTML → PNG via Chromium headless ───────────────────────────────────
render_png() {
  local html_file="$1"
  local png_file="$2"
  local width="$3"
  local height="$4"

  local url="file://${html_file}"

  if [[ "${LIVE_DATA}" == "true" ]]; then
    local qs
    qs="$(build_query_string)"
    [[ -n "${qs}" ]] && url="${url}?${qs}"
  fi

  echo "→ Rendering $(basename "${html_file}") at ${width}×${height}…"
  echo "  URL: ${url}"

  # --virtual-time-budget gives JS and CSS animations time to settle
  # --hide-scrollbars prevents scrollbar artefacts
  # --default-background-color=00000000 = RGBA transparent (alpha=0)
  "${CHROMIUM}" \
    --headless \
    --disable-gpu \
    --no-sandbox \
    --disable-setuid-sandbox \
    --hide-scrollbars \
    --screenshot="${png_file}" \
    --window-size="${width},${height}" \
    --virtual-time-budget=3000 \
    --default-background-color=00000000 \
    "${url}" 2>/dev/null

  if [[ -f "${png_file}" ]]; then
    echo "  ✓ PNG written: ${png_file}"
  else
    echo "  ✗ ERROR: PNG was not created. Check Chromium output above."
    exit 1
  fi
}

# ── ffmpeg overlay composite ──────────────────────────────────────────────────
composite_overlay() {
  local input="$1"
  local overlay_png="$2"
  local output="$3"
  local filter="$4"   # ffmpeg -filter_complex string

  echo "→ Compositing overlay onto video…"
  echo "  Input:  ${input}"
  echo "  Output: ${output}"

  ffmpeg -y \
    -i "${input}" \
    -i "${overlay_png}" \
    -filter_complex "${filter}" \
    -c:v libx264 -crf "${QUALITY}" -preset slow \
    -c:a copy \
    "${output}"

  echo "  ✓ Video written: ${output}"
}

# ── Corner position helper ────────────────────────────────────────────────────
corner_filter() {
  local overlay_png="$1"
  local pad="${POSITION_X}"  # padding from edge

  case "${CORNER}" in
    tl) echo "[0:v][1:v]overlay=${pad}:${pad}:format=auto" ;;
    tr) echo "[0:v][1:v]overlay=W-w-${pad}:${pad}:format=auto" ;;
    bl) echo "[0:v][1:v]overlay=${pad}:H-h-${pad}:format=auto" ;;
    br) echo "[0:v][1:v]overlay=W-w-${pad}:H-h-${pad}:format=auto" ;;
    *)  echo "[0:v][1:v]overlay=${pad}:${pad}:format=auto" ;;
  esac
}

# ─────────────────────────────────────────────────────────────────────────────
# MAIN
# ─────────────────────────────────────────────────────────────────────────────

case "${VARIANT}" in

  # ── Corner card (520×290) ─────────────────────────────────────────────────
  corner)
    render_png "${CORNER_HTML}" "${CORNER_PNG}" 520 290

    if [[ "${RENDER_ONLY}" == "true" ]]; then exit 0; fi
    if [[ -z "${INPUT_VIDEO}" ]]; then
      echo "INFO: No --input provided. PNG rendered; skipping ffmpeg composite."
      exit 0
    fi

    OUTPUT_VIDEO="${OUTPUT_VIDEO:-${SCRIPT_DIR}/output-corner.mp4}"
    FILTER="$(corner_filter "${CORNER_PNG}")"
    composite_overlay "${INPUT_VIDEO}" "${CORNER_PNG}" "${OUTPUT_VIDEO}" "${FILTER}"
    ;;

  # ── Lower third (1920×120) ────────────────────────────────────────────────
  lower-third)
    render_png "${LOWER_HTML}" "${LOWER_PNG}" 1920 120

    if [[ "${RENDER_ONLY}" == "true" ]]; then exit 0; fi
    if [[ -z "${INPUT_VIDEO}" ]]; then
      echo "INFO: No --input provided. PNG rendered; skipping ffmpeg composite."
      exit 0
    fi

    OUTPUT_VIDEO="${OUTPUT_VIDEO:-${SCRIPT_DIR}/output-lower-third.mp4}"
    # Pin to bottom of frame with POSITION_Y pixels gap
    FILTER="[0:v][1:v]overlay=0:H-h-${POSITION_Y}:format=auto"
    composite_overlay "${INPUT_VIDEO}" "${LOWER_PNG}" "${OUTPUT_VIDEO}" "${FILTER}"
    ;;

  # ── Sidebar (300×720) ─────────────────────────────────────────────────────
  sidebar)
    render_png "${SIDEBAR_HTML}" "${SIDEBAR_PNG}" 300 720

    if [[ "${RENDER_ONLY}" == "true" ]]; then exit 0; fi
    if [[ -z "${INPUT_VIDEO}" ]]; then
      echo "INFO: No --input provided. PNG rendered; skipping ffmpeg composite."
      exit 0
    fi

    OUTPUT_VIDEO="${OUTPUT_VIDEO:-${SCRIPT_DIR}/output-sidebar.mp4}"
    # Default: right edge, 20px pad, vertically centred
    FILTER="[0:v][1:v]overlay=W-w-${POSITION_X}:(H-h)/2:format=auto"
    composite_overlay "${INPUT_VIDEO}" "${SIDEBAR_PNG}" "${OUTPUT_VIDEO}" "${FILTER}"
    ;;

  *)
    echo "ERROR: Unknown variant '${VARIANT}'. Use: corner | lower-third | sidebar"
    exit 1
    ;;
esac

echo ""
echo "Done."


# ╔══════════════════════════════════════════════════════════════════════════╗
# ║  MANUAL REFERENCE — Copy-paste commands                                  ║
# ╠══════════════════════════════════════════════════════════════════════════╣
# ║                                                                          ║
# ║  1. RENDER PNGs only (all three variants):                               ║
# ║                                                                          ║
# ║  CHROMIUM="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" ║
# ║  "$CHROMIUM" --headless --disable-gpu --no-sandbox --hide-scrollbars \   ║
# ║    --screenshot=overlay-corner.png --window-size=520,290 \               ║
# ║    --default-background-color=00000000 \                                 ║
# ║    file://$(pwd)/overlay-card--corner.html                               ║
# ║                                                                          ║
# ║  "$CHROMIUM" --headless --disable-gpu --no-sandbox --hide-scrollbars \   ║
# ║    --screenshot=overlay-lower-third.png --window-size=1920,120 \         ║
# ║    --default-background-color=00000000 \                                 ║
# ║    file://$(pwd)/overlay-card--lower-third.html                          ║
# ║                                                                          ║
# ║  "$CHROMIUM" --headless --disable-gpu --no-sandbox --hide-scrollbars \   ║
# ║    --screenshot=overlay-sidebar.png --window-size=300,720 \              ║
# ║    --default-background-color=00000000 \                                 ║
# ║    file://$(pwd)/overlay-card--sidebar.html                              ║
# ║                                                                          ║
# ║  2. FFMPEG COMPOSITES:                                                   ║
# ║                                                                          ║
# ║  # Corner — top-left (20px pad):                                         ║
# ║  ffmpeg -i input.mp4 -i overlay-corner.png \                             ║
# ║    -filter_complex "[0:v][1:v]overlay=20:20:format=auto" \               ║
# ║    -c:v libx264 -crf 18 -c:a copy output-corner.mp4                     ║
# ║                                                                          ║
# ║  # Corner — bottom-right:                                                ║
# ║  ffmpeg -i input.mp4 -i overlay-corner.png \                             ║
# ║    -filter_complex "[0:v][1:v]overlay=W-w-20:H-h-20:format=auto" \       ║
# ║    -c:v libx264 -crf 18 -c:a copy output-corner-br.mp4                  ║
# ║                                                                          ║
# ║  # Lower third — flush to bottom edge:                                   ║
# ║  ffmpeg -i input.mp4 -i overlay-lower-third.png \                        ║
# ║    -filter_complex "[0:v][1:v]overlay=0:H-h:format=auto" \               ║
# ║    -c:v libx264 -crf 18 -c:a copy output-lower-third.mp4                ║
# ║                                                                          ║
# ║  # Sidebar — right edge, vertically centred:                             ║
# ║  ffmpeg -i input.mp4 -i overlay-sidebar.png \                            ║
# ║    -filter_complex "[0:v][1:v]overlay=W-w-20:(H-h)/2:format=auto" \      ║
# ║    -c:v libx264 -crf 18 -c:a copy output-sidebar.mp4                    ║
# ║                                                                          ║
# ║  3. LIVE DATA via URL params (render with Prometheus values):             ║
# ║                                                                          ║
# ║  URL="file://$(pwd)/overlay-card--corner.html"                           ║
# ║  URL+="?chilli_temp=31.2%C2%B0&chilli_soil=14%25&chilli_status=alert"   ║
# ║  URL+="&basil_temp=22.4%C2%B0&basil_status=monitor"                     ║
# ║  URL+="&alert_count=2&timestamp=09%3A41%3A22"                            ║
# ║  "$CHROMIUM" --headless ... --screenshot=overlay-corner.png "$URL"       ║
# ║                                                                          ║
# ║  4. N8N INTEGRATION HINT:                                                ║
# ║     HTTP Request node → Prometheus API → Set node to map values          ║
# ║     → Execute Command node running this script with env vars             ║
# ║     → Execute Command node running ffmpeg composite                      ║
# ║                                                                          ║
# ╚══════════════════════════════════════════════════════════════════════════╝
