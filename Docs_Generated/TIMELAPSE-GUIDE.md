# Timelapse Creation Guide

A step-by-step reference for generating timelapse videos in HerbHub365, from raw images through to publishing on the Jekyll site.

---

## Overview

The timelapse pipeline takes still images captured by the greenhouse camera, filters and encodes them into an MP4 using `ffmpeg`, and publishes the video to the Jekyll-based hub site.

```
Raw images (date-based folders on disk)
        ↓
scripts/make-timelapse.sh   (core engine — ffmpeg + optional ImageMagick)
        ↓
output .mp4 file
        ↓
hub/assets/video/           (served by Jekyll)
        ↓
hub/_posts/                 (blog post embeds the video)
```

---

## Prerequisites

### Running the script directly
- `ffmpeg` installed and available in `PATH`
- `imagemagick` (optional — only needed for brightness-based frame filtering)

### Running via Docker
- Docker and Docker Compose installed
- `docker/.env` configured (see [Docker method](#option-2--docker-recommended-for-production) below)

---

## Image Directory Structure

The script expects images to be organised in a root directory containing **date-named subdirectories**:

```
/home/andy/Pictures/timelapse/
├── 2026-03-24/
│   ├── 20260324_080000.jpg
│   ├── 20260324_090000.jpg
│   └── ...
├── 2026-03-25/
│   ├── 20260325_080000.jpg
│   └── ...
```

### Supported image formats
`.jpg` / `.jpeg` / `.png` / `.webp`

### Filename timestamp convention

Use `YYYYMMDD_HHMMSS` in the filename for accurate time-based filtering:

```
20260325_083000.jpg
```

If the filename does not match this pattern the script falls back to the file's modification time.

---

## Option 1 — Shell Script (quickest)

Run `scripts/make-timelapse.sh` directly from the repo root.

### Basic usage — all images, default output

```bash
scripts/make-timelapse.sh
```

Reads from `/home/andy/Pictures/timelapse` and writes `timelapse-YYYYMMDD-HHMMSS.mp4` in the current directory.

### Specify input directory and output file

```bash
scripts/make-timelapse.sh /home/andy/Pictures/timelapse ./hub/assets/video/timelapse.mp4
```

### Filter by date/time range

```bash
scripts/make-timelapse.sh \
  --from "2026-03-25 08:00:00" \
  --to   "2026-03-25 18:00:00" \
  /home/andy/Pictures/timelapse \
  ./hub/assets/video/timelapse.mp4
```

Accepted `--from` / `--to` formats:

| Format | Example |
|---|---|
| Date only | `2026-03-25` |
| Date and time | `2026-03-25 08:00:00` |
| Compact | `20260325 080000` |
| ISO 8601 | `2026-03-25T08:00:00` |

### Tune quality and speed with environment variables

```bash
INPUT_FPS=12 OUTPUT_FPS=24 CRF=18 MIN_BRIGHTNESS=0.05 \
  scripts/make-timelapse.sh /home/andy/Pictures/timelapse ./out.mp4
```

### Environment variable reference

| Variable | Default | Description |
|---|---|---|
| `INPUT_FPS` | `8` | Rate at which source images are fed into the encoder |
| `OUTPUT_FPS` | `30` | Frame rate of the output video |
| `CRF` | `23` | x264 quality factor — lower = better quality, larger file (18 is near-lossless) |
| `MIN_BRIGHTNESS` | `0` (disabled) | Skip frames darker than this value (0–1 scale). Requires ImageMagick |

---

## Option 2 — Docker (recommended for production)

The `timelapse-builder` service in `docker/docker-compose.yml` wraps the same script inside an Alpine Linux container with `ffmpeg` and ImageMagick pre-installed.

### 1. Configure `docker/.env`

Key variables to set:

```dotenv
TIMELAPSE_MODE=once                          # "once" for a single run, "loop" for recurring
TIMELAPSE_INTERVAL_SECONDS=3600              # seconds between builds when MODE=loop
TIMELAPSE_FROM=2026-03-25                    # optional — start of range
TIMELAPSE_TO=2026-03-26                      # optional — end of range
TIMELAPSE_OUTPUT_NAME=timelapse.mp4          # leave blank for a timestamped filename
TIMELAPSE_INPUT_PATH=/home/andy/Pictures/timelapse
TIMELAPSE_OUTPUT_PATH=../services/timelapse-builder/output
TIMELAPSE_INPUT_FPS=8
TIMELAPSE_OUTPUT_FPS=30
TIMELAPSE_CRF=23
TIMELAPSE_MIN_BRIGHTNESS=0
```

### 2. Run once (manual trigger)

```bash
docker compose -f docker/docker-compose.yml run --rm timelapse-builder
```

The output `.mp4` will appear in the path set by `TIMELAPSE_OUTPUT_PATH`.

### 3. Run on a recurring schedule (background service)

Set `TIMELAPSE_MODE=loop` and `TIMELAPSE_INTERVAL_SECONDS` in `.env`, then:

```bash
docker compose -f docker/docker-compose.yml up -d timelapse-builder
```

Check logs:

```bash
docker compose -f docker/docker-compose.yml logs -f timelapse-builder
```

Stop the service:

```bash
docker compose -f docker/docker-compose.yml stop timelapse-builder
```

---

## Publishing the Video to the Hub Site

### 1. Copy the MP4 to the assets folder

```bash
cp ./out.mp4 hub/assets/video/timelapse.mp4
```

> The Jekyll site serves all files under `hub/assets/` as static assets.

### 2. Create a new blog post

Add a file to `hub/_posts/` following the naming convention `YYYY-MM-DD-your-title.markdown`:

```bash
touch hub/_posts/2026-03-26-timelapse-march-26.markdown
```

Minimal post template:

```markdown
---
layout: post
title: "Timelapse – 26 March 2026"
date: 2026-03-26 18:00:00 +0000
categories: Herb Hub Update
---

Daytime timelapse for 26 March 2026.

<video width="640" height="360" controls>
  <source src="/assets/video/timelapse.mp4" type="video/mp4">
  Your browser does not support the video tag.
</video>
```

### 3. Build and preview the site locally (optional)

```bash
cd hub
bundle exec jekyll serve
```

Then open `http://localhost:4000`.

---

## Common Scenarios

### Daytime-only timelapse (skip dark/night frames)

```bash
MIN_BRIGHTNESS=0.05 scripts/make-timelapse.sh \
  /home/andy/Pictures/timelapse \
  ./hub/assets/video/timelapse.mp4
```

### Single day timelapse

```bash
scripts/make-timelapse.sh \
  --from "2026-03-25 00:00:00" \
  --to   "2026-03-25 23:59:59" \
  /home/andy/Pictures/timelapse \
  ./hub/assets/video/timelapse-2026-03-25.mp4
```

### High quality archive export

```bash
CRF=16 INPUT_FPS=10 OUTPUT_FPS=60 \
  scripts/make-timelapse.sh \
  /home/andy/Pictures/timelapse \
  ./archive-hq.mp4
```

### Quick low-bandwidth preview

```bash
CRF=30 INPUT_FPS=4 OUTPUT_FPS=24 \
  scripts/make-timelapse.sh \
  /home/andy/Pictures/timelapse \
  ./preview.mp4
```

---

## Troubleshooting

| Problem | Likely cause | Fix |
|---|---|---|
| `ffmpeg is required but was not found in PATH` | ffmpeg not installed | Install with `brew install ffmpeg` (macOS) or `apt install ffmpeg` (Linux) |
| `No images found under: /path` | Wrong input directory | Check `TIMELAPSE_INPUT_PATH` or the first positional argument |
| `No usable images found after filtering` | `--from`/`--to` range excludes everything, or `MIN_BRIGHTNESS` is too high | Widen the date range or lower `MIN_BRIGHTNESS` |
| `--from must be earlier than or equal to --to` | Date args swapped | Swap the `--from` and `--to` values |
| `Warning: could not determine timestamp for ...` | Filename doesn't match `YYYYMMDD_HHMMSS` pattern and `stat` failed | Rename files to the expected convention |
| `MIN_BRIGHTNESS is set but ImageMagick is not installed` | ImageMagick missing | Install ImageMagick or set `MIN_BRIGHTNESS=0` to disable |
| Docker output directory empty | `TIMELAPSE_OUTPUT_PATH` not mapped correctly | Check the volume mount in `docker/.env` |

---

## File Reference

| File | Purpose |
|---|---|
| `scripts/make-timelapse.sh` | Core timelapse build script |
| `services/timelapse-builder/entrypoint.sh` | Docker entrypoint — handles `once`/`loop` modes |
| `services/timelapse-builder/dockerfile` | Container definition (Alpine + ffmpeg + ImageMagick) |
| `docker/docker-compose.yml` | Production service definitions including `timelapse-builder` |
| `docker/.env` | Environment variable configuration for all services |
| `hub/assets/video/` | Where the published `.mp4` files live |
| `hub/_posts/` | Jekyll blog posts — embed videos here |
