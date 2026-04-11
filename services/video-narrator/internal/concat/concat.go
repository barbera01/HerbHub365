// Package concat stitches an intro, a main avatar video, and an outro into a
// single YouTube-ready MP4 using ffmpeg.
//
// All three inputs are normalised to 1920×1080 @ 29.97fps yuv420p H.264 +
// AAC stereo 48 kHz before concatenation, so mismatched codecs or frame sizes
// from the avatar video API will never cause a broken output.
//
// When ChromaKey is enabled the avatar segment's green screen is keyed out
// and composited over a configurable background before stitching.
package concat

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"HerbHub365/services/video-narrator/internal/config"
)

// imageExts is the set of file extensions treated as static images.
// Static images need -loop 1 so ffmpeg repeats them for the full segment duration.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".bmp": true, ".tiff": true, ".tif": true, ".webp": true,
}

// Stitch concatenates intro + main + outro into outputPath.
// It re-encodes all three segments to ensure a consistent YouTube-ready output.
// When cfg.ChromaKey.Enabled is true the avatar segment has its green screen
// removed and composited over the configured background.
func Stitch(ctx context.Context, cfg config.ConcatConfig, mainVideoPath, outputPath string) error {
	ffmpegPath := cfg.FFmpegPath
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	// Build input list and filter_complex together — they are coupled when
	// chromakey is active because the background source is an extra input.
	args, err := buildArgs(cfg, mainVideoPath, outputPath)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat failed: %w\n%s", err, trimOutput(string(out)))
	}
	return nil
}

// buildArgs constructs the full ffmpeg argument list.
func buildArgs(cfg config.ConcatConfig, mainVideoPath, outputPath string) ([]string, error) {
	const (
		w   = "1920"
		h   = "1080"
		fps = "30000/1001" // 29.97 fps
	)

	args := []string{"-y"}

	// ── inputs ────────────────────────────────────────────────────────────────
	// Inputs 0, 1, 2 are always intro / avatar / outro.
	args = append(args, "-i", cfg.IntroPath)
	args = append(args, "-i", mainVideoPath)
	args = append(args, "-i", cfg.OutroPath)

	// Optional input 3: background for chromakey compositing.
	bgInputIndex := -1
	if cfg.ChromaKey.Enabled {
		bgInputIndex = 3
		if cfg.ChromaKey.BGPath != "" {
			ext := strings.ToLower(filepath.Ext(cfg.ChromaKey.BGPath))
			if imageExts[ext] {
				// Static image — loop it so it covers the full avatar duration.
				args = append(args,
					"-loop", "1",
					"-framerate", fps,
					"-i", cfg.ChromaKey.BGPath,
				)
			} else {
				// Video — loop indefinitely.
				args = append(args,
					"-stream_loop", "-1",
					"-i", cfg.ChromaKey.BGPath,
				)
			}
		} else {
			// Solid colour via libavfilter lavfi device.
			lavfi := fmt.Sprintf("color=c=%s:size=%sx%s:rate=%s", cfg.ChromaKey.BGColor, w, h, fps)
			args = append(args, "-f", "lavfi", "-i", lavfi)
		}
	}

	// ── filter_complex ────────────────────────────────────────────────────────
	filterComplex := buildFilterComplex(cfg.ChromaKey, bgInputIndex, w, h, fps)
	args = append(args,
		"-filter_complex", filterComplex,
		"-map", "[vout]",
		"-map", "[aout]",
		// Video encode
		"-c:v", "libx264",
		"-preset", cfg.Preset,
		"-crf", strconv.Itoa(cfg.CRF),
		"-pix_fmt", "yuv420p",
		// Audio encode
		"-c:a", "aac",
		"-b:a", "192k",
		"-ar", "48000",
		// YouTube fast-start
		"-movflags", "+faststart",
		outputPath,
	)

	return args, nil
}

// buildFilterComplex returns the ffmpeg filter_complex string.
//
// When chromakey is disabled:
//
//	All three inputs are normalised (scale→pad→fps→format + aresample) and
//	concatenated: intro → avatar → outro.
//
// When chromakey is enabled:
//
//	Inputs 0 and 2 (intro/outro) are normalised as above.
//	Input 1 (avatar) has its green screen keyed out, composited over the
//	background source at bgInputIndex, then normalised like the others.
func buildFilterComplex(ck config.ChromaKeyConfig, bgInputIndex int, w, h, fps string) string {
	var sb strings.Builder

	// ── video normalisation ───────────────────────────────────────────────────
	normaliseV := func(inputIdx int, outLabel string) {
		sb.WriteString(fmt.Sprintf(
			"[%d:v]scale=%s:%s:force_original_aspect_ratio=decrease,"+
				"pad=%s:%s:(ow-iw)/2:(oh-ih)/2:color=black,"+
				"fps=%s,format=yuv420p[%s];",
			inputIdx, w, h, w, h, fps, outLabel,
		))
	}

	// Input 0 (intro) — always standard normalisation.
	normaliseV(0, "v0")

	// Input 1 (avatar) — chromakey path or standard.
	if ck.Enabled {
		// Step 1: scale avatar to target resolution FIRST, padding any
		// letterbox/pillarbox areas with the chroma-key colour so they are
		// removed along with the green screen in the next step.
		// This ensures the keyed layer is always exactly w×h before compositing,
		// fixing both wrong-position and ghost-size artefacts.
		sb.WriteString(fmt.Sprintf(
			"[1:v]scale=%s:%s:force_original_aspect_ratio=decrease,"+
				"pad=%s:%s:(ow-iw)/2:(oh-ih)/2:color=%s,fps=%s[scaled1];",
			w, h, w, h, ck.Color, fps,
		))
		// Step 2: apply chromakey to the full-resolution avatar → keyed YUVA stream.
		// Optional despill removes green colour cast from edges (hair, shoulders).
		if ck.Despill > 0 {
			sb.WriteString(fmt.Sprintf(
				"[scaled1]chromakey=color=%s:similarity=%.2f:blend=%.2f,despill=type=green:mix=%.2f[ck1];",
				ck.Color, ck.Similarity, ck.Blend, ck.Despill,
			))
		} else {
			sb.WriteString(fmt.Sprintf(
				"[scaled1]chromakey=color=%s:similarity=%.2f:blend=%.2f[ck1];",
				ck.Color, ck.Similarity, ck.Blend,
			))
		}
		// Step 3: normalise the background source to w×h.
		sb.WriteString(fmt.Sprintf(
			"[%d:v]scale=%s:%s:force_original_aspect_ratio=decrease,"+
				"pad=%s:%s:(ow-iw)/2:(oh-ih)/2:color=black,"+
				"fps=%s,setsar=1[bg1];",
			bgInputIndex, w, h, w, h, fps,
		))
		// Step 4: composite keyed avatar (w×h) over background (w×h) at origin,
		// then finalise fps and pixel format.
		sb.WriteString(fmt.Sprintf(
			"[bg1][ck1]overlay=0:0:shortest=1,fps=%s,format=yuv420p[v1];",
			fps,
		))
	} else {
		normaliseV(1, "v1")
	}

	// Input 2 (outro) — always standard normalisation.
	normaliseV(2, "v2")

	// ── audio normalisation ───────────────────────────────────────────────────
	for i := 0; i < 3; i++ {
		sb.WriteString(fmt.Sprintf(
			"[%d:a]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo[a%d];",
			i, i,
		))
	}

	// ── concatenate ───────────────────────────────────────────────────────────
	sb.WriteString("[v0][a0][v1][a1][v2][a2]concat=n=3:v=1:a=1[vout][aout]")

	return sb.String()
}

// trimOutput returns the last 2000 characters of ffmpeg output to keep error
// messages readable without flooding the log.
func trimOutput(s string) string {
	if len(s) <= 2000 {
		return s
	}
	return "…" + s[len(s)-2000:]
}
