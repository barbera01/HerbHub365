#!/usr/bin/env bash
#
# test-chromakey.sh — Rapid iteration on chromakey filter parameters.
#
# Composites a green-screen avatar MP4 over a background image/video
# using the EXACT same ffmpeg filter chain as concat.go (minus intro/outro).
#
# Usage:
#   ./test-chromakey.sh <avatar.mp4> <background.jpeg> <output.mp4> [options]
#
# Options (all optional, sensible defaults provided):
#   --color       Chroma key color (default: 0x19AB3B)
#   --similarity  Chromakey similarity 0.0-1.0 (default: 0.08)
#   --blend       Chromakey blend 0.0-1.0 (default: 0.0)
#   --despill     Despill strength 0.0-1.0 (default: 0 = off)
#   --ffmpeg      Path to ffmpeg (default: ffmpeg)
#   --dry-run     Print the ffmpeg command but don't run it
#
# Examples:
#   # Basic usage with defaults (tight key, no blend)
#   ./test-chromakey.sh avatar.mp4 eve_home.jpeg out.mp4
#
#   # Add despill to remove green fringe from hair
#   ./test-chromakey.sh avatar.mp4 eve_home.jpeg out.mp4 --despill 1.0
#
#   # Slightly wider key with despill
#   ./test-chromakey.sh avatar.mp4 eve_home.jpeg out.mp4 --similarity 0.12 --despill 0.5
#
#   # Just print the command
#   ./test-chromakey.sh avatar.mp4 eve_home.jpeg out.mp4 --dry-run

set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Defaults — tuned for best results (tight key, no blend, despill off by default)
# ─────────────────────────────────────────────────────────────────────────────
COLOR="0x19AB3B"
SIMILARITY="0.08"
BLEND="0.0"
DESPILL="0"
FFMPEG="ffmpeg"
DRY_RUN=false

# Output specs (match concat.go)
W=1920
H=1080
FPS="30000/1001"

# ─────────────────────────────────────────────────────────────────────────────
# Parse arguments
# ─────────────────────────────────────────────────────────────────────────────
usage() {
    echo "Usage: $0 <avatar.mp4> <background.jpeg> <output.mp4> [options]"
    echo ""
    echo "Options:"
    echo "  --color <hex>       Chroma key color (default: 0x19AB3B)"
    echo "  --similarity <0-1>  Chromakey similarity (default: 0.08)"
    echo "  --blend <0-1>       Chromakey blend (default: 0.0)"
    echo "  --despill <0-1>     Despill strength to remove green fringe (default: 0 = off)"
    echo "  --ffmpeg <path>     Path to ffmpeg (default: ffmpeg)"
    echo "  --dry-run           Print command without executing"
    exit 1
}

if [[ $# -lt 3 ]]; then
    usage
fi

AVATAR="$1"
BACKGROUND="$2"
OUTPUT="$3"
shift 3

while [[ $# -gt 0 ]]; do
    case "$1" in
        --color)
            COLOR="$2"
            shift 2
            ;;
        --similarity)
            SIMILARITY="$2"
            shift 2
            ;;
        --blend)
            BLEND="$2"
            shift 2
            ;;
        --despill)
            DESPILL="$2"
            shift 2
            ;;
        --ffmpeg)
            FFMPEG="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

# ─────────────────────────────────────────────────────────────────────────────
# Validate inputs
# ─────────────────────────────────────────────────────────────────────────────
if [[ ! -f "$AVATAR" ]]; then
    echo "Error: Avatar file not found: $AVATAR"
    exit 1
fi

if [[ ! -f "$BACKGROUND" ]]; then
    echo "Error: Background file not found: $BACKGROUND"
    exit 1
fi

# ─────────────────────────────────────────────────────────────────────────────
# Detect if background is image or video
# ─────────────────────────────────────────────────────────────────────────────
BG_EXT="${BACKGROUND##*.}"
BG_EXT_LOWER=$(echo "$BG_EXT" | tr '[:upper:]' '[:lower:]')

BG_INPUT_ARGS=()
case "$BG_EXT_LOWER" in
    jpg|jpeg|png|bmp|tiff|tif|webp)
        # Static image — loop it
        BG_INPUT_ARGS=(-loop 1 -framerate "$FPS" -i "$BACKGROUND")
        ;;
    *)
        # Video — loop indefinitely
        BG_INPUT_ARGS=(-stream_loop -1 -i "$BACKGROUND")
        ;;
esac

# ─────────────────────────────────────────────────────────────────────────────
# Build filter_complex
# ─────────────────────────────────────────────────────────────────────────────
# Input 0: avatar (green screen)
# Input 1: background
#
# Step 1: Scale avatar to target res, pad with chroma color
# Step 2: Apply chromakey
# Step 2b (optional): Apply despill to remove green color cast from edges
# Step 3: Scale/pad background
# Step 4: Overlay keyed avatar on background

# Build the chromakey + optional despill chain
if [[ "$DESPILL" != "0" && "$DESPILL" != "0.0" ]]; then
    # despill filter removes green color cast from semi-transparent edges (hair)
    # type=green targets green spill specifically
    CHROMA_CHAIN="chromakey=color=${COLOR}:similarity=${SIMILARITY}:blend=${BLEND},despill=type=green:mix=${DESPILL}"
else
    CHROMA_CHAIN="chromakey=color=${COLOR}:similarity=${SIMILARITY}:blend=${BLEND}"
fi

FILTER_COMPLEX=$(cat <<EOF
[0:v]scale=${W}:${H}:force_original_aspect_ratio=decrease,pad=${W}:${H}:(ow-iw)/2:(oh-ih)/2:color=${COLOR},fps=${FPS}[scaled];
[scaled]${CHROMA_CHAIN}[ck];
[1:v]scale=${W}:${H}:force_original_aspect_ratio=decrease,pad=${W}:${H}:(ow-iw)/2:(oh-ih)/2:color=black,fps=${FPS},setsar=1[bg];
[bg][ck]overlay=0:0:shortest=1,fps=${FPS},format=yuv420p[vout]
EOF
)

# ─────────────────────────────────────────────────────────────────────────────
# Build full ffmpeg command
# ─────────────────────────────────────────────────────────────────────────────
CMD=(
    "$FFMPEG" -y
    -i "$AVATAR"
    "${BG_INPUT_ARGS[@]}"
    -filter_complex "$FILTER_COMPLEX"
    -map "[vout]"
    -map "0:a?"
    -c:v libx264
    -preset medium
    -crf 18
    -pix_fmt yuv420p
    -c:a aac
    -b:a 192k
    -ar 48000
    -movflags +faststart
    "$OUTPUT"
)

# ─────────────────────────────────────────────────────────────────────────────
# Print and optionally run
# ─────────────────────────────────────────────────────────────────────────────
echo "═══════════════════════════════════════════════════════════════════════════"
echo "Chromakey Test"
echo "═══════════════════════════════════════════════════════════════════════════"
echo "Avatar:     $AVATAR"
echo "Background: $BACKGROUND"
echo "Output:     $OUTPUT"
echo ""
echo "Parameters:"
echo "  Color:      $COLOR"
echo "  Similarity: $SIMILARITY"
echo "  Blend:      $BLEND"
echo "  Despill:    $DESPILL"
echo ""
echo "Filter chain:"
echo "$FILTER_COMPLEX"
echo ""
echo "Full command:"
echo "${CMD[@]}"
echo "═══════════════════════════════════════════════════════════════════════════"

if [[ "$DRY_RUN" == true ]]; then
    echo ""
    echo "(dry-run mode — command not executed)"
    exit 0
fi

echo ""
echo "Running ffmpeg..."
echo ""

"${CMD[@]}"

echo ""
echo "Done! Output: $OUTPUT"
echo ""
echo "Quick check with ffprobe:"
"${FFMPEG%ffmpeg}ffprobe" -v error -select_streams v:0 \
    -show_entries stream=width,height,r_frame_rate,pix_fmt,codec_name \
    -of csv=p=0 "$OUTPUT" 2>/dev/null || true
