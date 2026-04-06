---
layout: post
title: "How the Herb Hub timelapse service works"
date: 2026-03-25 20:00:00 +0000
categories: Platform Update
audio_url: /assets/audio/blog/2026-03-25-how-the-herb-hub-timelapse-service-works.mp3
---

The Herb Hub platform includes a dedicated component designed to transform raw image captures into polished MP4 video sequences. This service, often referred to as the timelapse builder, allows users and automated systems to generate time-lapsed footage directly from stored images without requiring manual intervention for every new batch of photos. It is an integral part of the broader monitoring infrastructure that supports long-term observation projects on the platform.

## Image Selection and Filtering Logic

The core engine responsible for generating videos scans a designated input directory containing date-based subdirectories. This script identifies image files with common extensions such as JPG, JPEG, PNG, or WebP to construct the sequence. Before rendering begins, an intelligent filtering process ensures only relevant frames are included in the final output. Users can specify temporal boundaries using start and end datetime arguments; any images falling outside this requested window are automatically excluded from the build process.

In addition to time-based selection, the system supports brightness thresholding when ImageMagick is available within the environment. This feature allows for the automatic skipping of frames that fall below a specific darkness level during video generation. By filtering out these dark moments, such as those occurring at night or in poorly lit conditions, the resulting timelapse maintains consistent visual quality throughout its duration without requiring manual frame curation.

## Container Architecture and Dependencies

The service operates within an isolated Docker container built on Alpine Linux 3.20 to ensure a lightweight yet functional runtime environment. Upon initialization, this image installs essential system utilities including Bash, coreutils, findutils, and ImageMagick alongside the primary media processing tool required for video encoding: ffmpeg. The entrypoint script manages the lifecycle of the build process, supporting both single-run execution modes suitable for scheduled tasks or manual triggers, as well as a continuous loop mode that rebuilds videos at configurable intervals to keep content fresh without constant user interaction.

## Output Configuration and Video Encoding Parameters

When generating video files, the service relies on environment variables passed into the container to define specific quality characteristics of the output stream. The system defaults to an input frame rate derived from image capture frequency while encoding a final video typically at 30 frames per second for smooth playback. Quality is controlled through a configurable compression factor that balances file size against visual fidelity, with default settings tuned to produce high-quality results suitable for web distribution and archival storage.

The resulting MP4 files are written directly into an output directory specified during container deployment or configuration. Each generated video includes metadata reflecting the creation timestamp in its filename unless a custom name is provided via environment variables. The service also employs specific encoding flags that enable fast start streaming, allowing viewers to begin watching content immediately after downloading without waiting for the entire file to buffer before playback begins on compatible devices and browsers.
