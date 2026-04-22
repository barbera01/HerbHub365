---
layout: post
title: "The Technology Behind Herb Hub 365"
date: 2026-04-22 12:00:00 +0000
categories: Platform Update
image: /assets/images/herbhub-home.png
image_alt: "The Herb Hub 365 hardware setup — Raspberry Pi, sensors, and relay board"
---

Herb Hub 365 has grown from a simple greenhouse monitor into a small but fairly involved software platform. This post is a transparent look at everything that goes into running it — the languages, services, tools, and external integrations that combine to keep the greenhouse observed, narrated, and published each day.

<div style="margin: 2.5rem 0; padding: 1.75rem; background: var(--foam); border-radius: var(--radius-md); border: 1px solid var(--border);">
  <p style="font-family: 'Lora', serif; color: var(--forest); font-size: 0.75rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; margin: 0 0 1.25rem 0;">Stack at a Glance</p>

  <div style="margin-bottom: 1.1rem;">
    <p style="font-size: 0.68rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.1em; color: var(--ink-muted); margin: 0 0 0.5rem 0;">Languages &amp; Runtimes</p>
    <div style="display: flex; flex-wrap: wrap; gap: 7px;">
      <span style="display:inline-flex;align-items:center;gap:6px;background:#00ADD8;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><circle cx="12" cy="12" r="11" fill="white" opacity="0.25"/><text x="12" y="16" text-anchor="middle" fill="white" font-size="10" font-weight="900" font-family="sans-serif">Go</text></svg>
        Go 1.23
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#CC342D;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="white" xmlns="http://www.w3.org/2000/svg"><polygon points="12,2 20,7 20,17 12,22 4,17 4,7"/></svg>
        Ruby 3.3
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#3572A5;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><ellipse cx="12" cy="8" rx="7" ry="4" stroke="white" stroke-width="2"/><path d="M5 8 Q5 20 12 20 Q19 20 19 8" stroke="white" stroke-width="2" fill="none"/></svg>
        Python 3
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#1a1a1a;color:#4af626;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;font-family:monospace;">
        &gt;_ Bash
      </span>
    </div>
  </div>

  <div style="margin-bottom: 1.1rem;">
    <p style="font-size: 0.68rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.1em; color: var(--ink-muted); margin: 0 0 0.5rem 0;">Frontend &amp; Content</p>
    <div style="display: flex; flex-wrap: wrap; gap: 7px;">
      <span style="display:inline-flex;align-items:center;gap:6px;background:#CC0000;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><rect x="8" y="2" width="8" height="16" rx="2" fill="white" opacity="0.9"/><ellipse cx="12" cy="20" rx="6" ry="4" fill="white" opacity="0.9"/></svg>
        Jekyll 4.3
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#CF649A;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M4 4 L12 20 L20 4" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/></svg>
        Sass
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#4a7c59;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M4 6 Q12 2 20 6 Q12 10 4 6Z" fill="white" opacity="0.9"/><path d="M4 12 Q12 8 20 12 Q12 16 4 12Z" fill="white" opacity="0.7"/><path d="M4 18 Q12 14 20 18 Q12 22 4 18Z" fill="white" opacity="0.5"/></svg>
        Liquid
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#0078D4;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><polygon points="4,20 12,4 20,20" fill="none" stroke="white" stroke-width="2.5" stroke-linejoin="round"/></svg>
        Azure SWA
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#2088FF;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><circle cx="12" cy="12" r="9" stroke="white" stroke-width="2"/><path d="M8 12 L16 12 M12 8 L12 16" stroke="white" stroke-width="2" stroke-linecap="round"/></svg>
        GitHub Actions
      </span>
    </div>
  </div>

  <div style="margin-bottom: 1.1rem;">
    <p style="font-size: 0.68rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.1em; color: var(--ink-muted); margin: 0 0 0.5rem 0;">Infrastructure &amp; Messaging</p>
    <div style="display: flex; flex-wrap: wrap; gap: 7px;">
      <span style="display:inline-flex;align-items:center;gap:6px;background:#2496ED;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><rect x="3" y="10" width="5" height="5" rx="1" fill="white"/><rect x="10" y="10" width="5" height="5" rx="1" fill="white"/><rect x="17" y="10" width="5" height="5" rx="1" fill="white"/><rect x="6" y="4" width="5" height="5" rx="1" fill="white" opacity="0.8"/><rect x="13" y="4" width="5" height="5" rx="1" fill="white" opacity="0.8"/><path d="M3 18 Q5 22 12 22 Q19 22 21 18" stroke="white" stroke-width="2" fill="none" stroke-linecap="round"/></svg>
        Docker
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#24A1C1;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M4 12 L10 6 L20 12 L10 18 Z" stroke="white" stroke-width="2" fill="none" stroke-linejoin="round"/><circle cx="10" cy="12" r="2" fill="white"/></svg>
        Traefik
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#FF6600;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><circle cx="9" cy="8" r="4" stroke="white" stroke-width="2"/><path d="M5 8 Q5 4 9 4" stroke="white" stroke-width="0" fill="white" opacity="0"/><ellipse cx="9" cy="6" rx="2" ry="1.5" fill="white" opacity="0.6"/><path d="M13 12 Q18 10 19 14 Q20 18 15 20 L7 20 Q4 20 4 17 Q4 14 8 13" stroke="white" stroke-width="1.5" fill="none" stroke-linecap="round"/></svg>
        RabbitMQ
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#5B4B8A;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><circle cx="12" cy="12" r="3" fill="white"/><path d="M12 3 L12 7 M12 17 L12 21 M3 12 L7 12 M17 12 L21 12" stroke="white" stroke-width="2" stroke-linecap="round"/></svg>
        Cronicle
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#E6522C;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M12 3 Q8 7 9 11 Q6 9 7 14 Q5 13 6 17 Q8 22 12 22 Q16 22 18 17 Q19 13 17 14 Q18 9 15 11 Q16 7 12 3Z" fill="white"/></svg>
        Prometheus
      </span>
    </div>
  </div>

  <div style="margin-bottom: 1.1rem;">
    <p style="font-size: 0.68rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.1em; color: var(--ink-muted); margin: 0 0 0.5rem 0;">AI &amp; Media</p>
    <div style="display: flex; flex-wrap: wrap; gap: 7px;">
      <span style="display:inline-flex;align-items:center;gap:6px;background:#1a1a1a;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><ellipse cx="12" cy="14" rx="7" ry="5" stroke="white" stroke-width="2"/><path d="M6 13 Q6 8 12 8 Q18 8 18 13" stroke="white" stroke-width="2" fill="none"/><line x1="12" y1="8" x2="12" y2="4" stroke="white" stroke-width="2" stroke-linecap="round"/></svg>
        Ollama
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#007808;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><rect x="2" y="6" width="20" height="12" rx="2" stroke="white" stroke-width="2"/><line x1="6" y1="6" x2="6" y2="18" stroke="white" stroke-width="1.5"/><line x1="10" y1="6" x2="10" y2="18" stroke="white" stroke-width="1.5"/><line x1="14" y1="6" x2="14" y2="18" stroke="white" stroke-width="1.5"/><line x1="18" y1="6" x2="18" y2="18" stroke="white" stroke-width="1.5"/></svg>
        ffmpeg
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#7B5EA7;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M12 4 Q6 8 6 12 Q6 18 12 20 Q18 18 18 12 Q18 8 12 4Z" stroke="white" stroke-width="2" fill="none"/><circle cx="12" cy="12" r="3" fill="white"/></svg>
        Kokoro TTS
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#76B900;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><rect x="2" y="4" width="20" height="16" rx="2" fill="none" stroke="white" stroke-width="2"/><line x1="7" y1="4" x2="7" y2="20" stroke="white" stroke-width="1.5"/><line x1="12" y1="4" x2="12" y2="20" stroke="white" stroke-width="1.5"/><line x1="17" y1="4" x2="17" y2="20" stroke="white" stroke-width="1.5"/></svg>
        NVIDIA NVENC
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#FF0000;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><rect x="2" y="5" width="20" height="14" rx="3" fill="white" opacity="0.25"/><polygon points="10,9 10,15 16,12" fill="white"/></svg>
        YouTube API
      </span>
    </div>
  </div>

  <div>
    <p style="font-size: 0.68rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.1em; color: var(--ink-muted); margin: 0 0 0.5rem 0;">Cloud &amp; IaC</p>
    <div style="display: flex; flex-wrap: wrap; gap: 7px;">
      <span style="display:inline-flex;align-items:center;gap:6px;background:#0078D4;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><polygon points="12,3 21,20 3,20" stroke="white" stroke-width="2" fill="none" stroke-linejoin="round"/></svg>
        Azure
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#7B42BC;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><polygon points="12,3 21,8 21,16 12,21 3,16 3,8" stroke="white" stroke-width="2" fill="none" stroke-linejoin="round"/><polygon points="12,8 17,11 17,16 12,19 7,16 7,11" fill="white" opacity="0.4"/></svg>
        Terraform
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#4a7c59;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M3 17 L7 7 L12 14 L17 7 L21 17" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"/></svg>
        Let's Encrypt
      </span>
      <span style="display:inline-flex;align-items:center;gap:6px;background:#C0392B;color:white;padding:5px 13px;border-radius:20px;font-size:0.78rem;font-weight:700;">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><circle cx="12" cy="12" r="9" stroke="white" stroke-width="2"/><path d="M8 10 Q8 7 12 7 Q16 7 16 10 Q16 12 12 14" stroke="white" stroke-width="2" stroke-linecap="round" fill="none"/><circle cx="12" cy="17" r="1.5" fill="white"/></svg>
        Raspberry Pi
      </span>
    </div>
  </div>
</div>

## Languages and Runtimes

The platform is built across four languages. Go forms the backbone of all eight microservices, running on version 1.23.4 for most services and 1.25.0 for the watering controller. Go was chosen primarily for its straightforward concurrency model, fast startup time in containers, and the ability to ship self-contained static binaries with no runtime dependencies.

The public-facing site is generated by Jekyll, running on Ruby 3.3. A small Python 3 script handles low-level GPIO relay control for the physical watering hardware, using the `gpiod` library to communicate directly with the Raspberry Pi's GPIO pins. Bash scripts tie together scheduled tasks, queue setup, and sensor data collection at the edges of the system.

## The Eight Microservices

Each service has a single, clearly bounded responsibility.

<div style="background: var(--foam); border-left: 4px solid var(--leaf); border-radius: 0 var(--radius-sm) var(--radius-sm) 0; padding: 1.25rem 1.5rem; margin: 1.5rem 0;">
  <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 12px;">
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">timelapse-builder</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">Image → MP4 · ffmpeg · stdlib only</p>
    </div>
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">video-narrator</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">MuseTalk · Kokoro TTS · NVENC/x264</p>
    </div>
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">video-publisher</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">YouTube Data API v3 · OAuth 2.0</p>
    </div>
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">llm-service</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">Ollama · Gemma 4 · stdlib only</p>
    </div>
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">blog-poster</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">Prometheus · LLM · Git · RabbitMQ</p>
    </div>
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">tts-narrator</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">Kokoro TTS · MP3 · cron</p>
    </div>
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">herbhub-manager</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">Orchestration · RabbitMQ · stdlib only</p>
    </div>
    <div style="background: white; border-radius: 10px; padding: 12px 14px; border: 1px solid var(--border);">
      <code style="font-size: 0.78rem; color: var(--leaf); font-weight: 700;">watering</code>
      <p style="font-size: 0.75rem; color: var(--ink-muted); margin: 4px 0 0 0; line-height: 1.4;">GPIO · Prometheus · RabbitMQ · Go 1.25</p>
    </div>
  </div>
</div>

**timelapse-builder** scans a directory of timestamped images, applies optional date range and brightness filters, and calls ffmpeg to produce an MP4. It exposes a REST API on port 8082 and enforces a single concurrent build at any time. It has no external Go dependencies — the entire service uses the standard library only.

**video-narrator** takes a script and produces a talking-head video by coordinating MuseTalk for avatar lip-sync and Kokoro TTS for voice synthesis. On machines with a compatible NVIDIA GPU it uses ffmpeg's h264_nvenc encoder for significantly faster output; on CPU-only machines it falls back to libx264.

**video-publisher** handles the YouTube side of things. Once a video is ready it uses the YouTube Data API v3 via Google's official Go client, including OAuth 2.0 authentication, to upload the file with the correct title, description, tags, and privacy settings.

**llm-service** is a thin HTTP wrapper around Ollama, the local LLM inference server. It abstracts model selection and request formatting so that other services can generate text without caring which model is running underneath. The default model is Gemma 4.

**blog-poster** is the most compositional service. It queries Prometheus for recent sensor metrics, requests a draft from llm-service, pulls a matching timelapse image, assembles the final Markdown post, and commits it directly to the Jekyll repository so that the site redeploys automatically.

**tts-narrator** converts written blog posts into audio narrations by sending requests to the Kokoro TTS API, producing MP3 files that are surfaced via the site's audio player.

**herbhub-manager** acts as the central coordinator, watching for new video outputs over RabbitMQ and triggering associated blog post generation. It exposes a small HTTP API on port 8080.

**watering** monitors soil moisture sensors via Prometheus, publishes telemetry to RabbitMQ, and triggers GPIO relay pulses through the Python relay wrapper when plants need watering.

## Messaging and Scheduling

Services communicate asynchronously through RabbitMQ, running the 3-management image so the full management UI is always available. The primary queues are `sensor.snapshots`, which carries telemetry data from the greenhouse, and `video.produced`, which signals that a new video is ready for downstream processing. A dead-letter queue sits alongside the main video queue to catch failed deliveries.

Scheduled work is handled by Cronicle (v1.14.2), a self-hosted web-based job scheduler. It drives the recurring sensor snapshots, timelapse builds, and nightly post generation without requiring cron entries spread across multiple machines.

## Infrastructure and Networking

All services run as Docker containers, defined in a single Compose file with an optional GPU variant for the video narrator. Traefik sits in front of everything as the reverse proxy, handling HTTPS termination with certificates provisioned automatically via Let's Encrypt. Each internal service is reachable through a subdomain — for example, `timelapse.herbhub365.com` routes directly to the timelapse builder's API.

Container and system metrics are collected by cAdvisor and scraped by a Prometheus agent running in agent mode, which forwards all data to a remote Prometheus instance at the home lab. A Homepage dashboard gives a single-pane view of service health.

## The Public Site

The site itself is a Jekyll 4.3.4 static site using the Minima theme with a heavily customised layout and CSS. It is deployed to Azure Static Web Apps via a GitHub Actions workflow that triggers on every push to main. The build and deployment typically completes in under two minutes. Content Security Policy headers are configured in `staticwebapp.config.json` and applied at the CDN edge.

## External Services and APIs

Beyond the self-hosted stack, the platform touches a small number of external services. YouTube receives finished videos through the Data API. Kokoro TTS and MuseTalk are hosted on the home lab network rather than the public internet. Azure hosts the static site and the Terraform state backend. Prometheus remote write carries metrics off-device for longer-term retention.

Secrets — YouTube OAuth tokens, Git personal access tokens, RabbitMQ credentials, and SSH keys for Cronicle — are injected via environment variables at container start rather than being baked into images.

## Media Processing

ffmpeg does the heavy lifting for all video work: compositing timelapse sequences, concatenating narrator segments with intro and outro clips, and controlling output quality through CRF settings. ImageMagick is used selectively for brightness analysis during timelapse frame filtering, though the plan is to eventually replace that with ffmpeg's native blackframe filter to avoid spawning a separate process per image.

## Infrastructure as Code

The Azure infrastructure — resource groups and the static web app — is defined in Terraform using the azurerm provider, with state stored in an Azure Storage Account. This keeps the cloud footprint reproducible and auditable, even though it currently covers a relatively small surface area.

---

The stack is intentionally practical rather than fashionable. Go for services, Jekyll for content, Docker Compose for orchestration, and a preference for self-hosted tooling wherever it makes sense. As the platform evolves the aim is to keep the dependency surface small and the individual components easy to reason about in isolation.
