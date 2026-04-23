---
layout: post
title: "The Hardware Behind Herb Hub 365"
date: 2026-04-22 14:00:00 +0000
categories: Platform Update
image: /assets/images/thehub.png
image_alt: "The Herb Hub 365 hardware control board — Raspberry Pi 400, relay module, and sensor wiring"
---

Every data point, timelapse frame, and narrated video that appears on this site starts with a piece of physical hardware sitting in or near the greenhouse. This post is a tour of the machines, boards, and sensors that make up the Herb Hub 365 infrastructure — what each one does, why it was chosen, and how they connect.

<div style="margin: 2.5rem 0; padding: 1.75rem; background: var(--foam); border-radius: var(--radius-md); border: 1px solid var(--border);">
  <p style="font-family: 'Lora', serif; color: var(--forest); font-size: 0.75rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; margin: 0 0 1.25rem 0;">The Fleet at a Glance</p>

  <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 14px;">

    <div style="background: white; border-radius: var(--radius-sm); border: 1px solid var(--border); border-top: 4px solid #C13B3B; padding: 14px 16px;">
      <div style="display:flex; align-items:center; gap:10px; margin-bottom:8px;">
        <svg width="28" height="28" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect width="48" height="48" rx="10" fill="#C13B3B"/>
          <rect x="8" y="16" width="32" height="20" rx="3" fill="white" opacity="0.15"/>
          <circle cx="14" cy="26" r="3" fill="white"/>
          <circle cx="24" cy="26" r="3" fill="white"/>
          <circle cx="34" cy="26" r="3" fill="white"/>
          <rect x="18" y="12" width="12" height="4" rx="1" fill="white" opacity="0.7"/>
        </svg>
        <div>
          <span style="font-family: monospace; font-size: 0.9rem; font-weight: 800; color: #1a3d2b; display: block;">hh-01</span>
          <p style="font-size: 0.7rem; color: #4a5e52; margin: 1px 0 0 0;">Image Capture</p>
        </div>
      </div>
      <p style="font-size: 0.75rem; color: #1e2e22; margin: 0; line-height: 1.5;">Raspberry Pi 3B · Cortex-A53 · 1 GB RAM<br>Pi AI Camera · WiFi · Debian 13</p>
    </div>

    <div style="background: white; border-radius: var(--radius-sm); border: 1px solid var(--border); border-top: 4px solid #2d7a4f; padding: 14px 16px;">
      <div style="display:flex; align-items:center; gap:10px; margin-bottom:8px;">
        <svg width="28" height="28" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect width="48" height="48" rx="10" fill="#2d7a4f"/>
          <rect x="8" y="18" width="32" height="18" rx="3" fill="white" opacity="0.15"/>
          <rect x="12" y="22" width="6" height="10" rx="1" fill="white"/>
          <rect x="21" y="22" width="6" height="10" rx="1" fill="white"/>
          <rect x="30" y="22" width="6" height="10" rx="1" fill="white"/>
          <path d="M10 14 Q24 10 38 14" stroke="white" stroke-width="2" stroke-linecap="round" fill="none" opacity="0.7"/>
        </svg>
        <div>
          <span style="font-family: monospace; font-size: 0.9rem; font-weight: 800; color: #1a3d2b; display: block;">hh-02</span>
          <p style="font-size: 0.7rem; color: #4a5e52; margin: 1px 0 0 0;">Hardware Control</p>
        </div>
      </div>
      <p style="font-size: 0.75rem; color: #1e2e22; margin: 0; line-height: 1.5;">Raspberry Pi 3B · Cortex-A53 · 1 GB RAM<br>Relays · Sensors · Docker · GPIO · WiFi</p>
    </div>

    <div style="background: white; border-radius: var(--radius-sm); border: 1px solid var(--border); border-top: 4px solid #00ADD8; padding: 14px 16px;">
      <div style="display:flex; align-items:center; gap:10px; margin-bottom:8px;">
        <svg width="28" height="28" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect width="48" height="48" rx="10" fill="#00ADD8"/>
          <rect x="6" y="14" width="36" height="22" rx="3" fill="white" opacity="0.2"/>
          <rect x="10" y="18" width="28" height="14" rx="2" fill="white" opacity="0.7"/>
          <rect x="14" y="36" width="20" height="3" rx="1" fill="white" opacity="0.5"/>
          <rect x="18" y="39" width="12" height="2" rx="1" fill="white" opacity="0.5"/>
        </svg>
        <div>
          <span style="font-family: monospace; font-size: 0.9rem; font-weight: 800; color: #1a3d2b; display: block;">hh-03</span>
          <p style="font-size: 0.7rem; color: #4a5e52; margin: 1px 0 0 0;">Orchestration &amp; Compute</p>
        </div>
      </div>
      <p style="font-size: 0.75rem; color: #1e2e22; margin: 0; line-height: 1.5;">Raspberry Pi 400 · Cortex-A72 · 4 GB RAM<br>Docker · NFS · 118 GB SD · Ethernet</p>
    </div>

    <div style="background: white; border-radius: var(--radius-sm); border: 1px solid var(--border); border-top: 4px solid #76B900; padding: 14px 16px;">
      <div style="display:flex; align-items:center; gap:10px; margin-bottom:8px;">
        <svg width="28" height="28" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect width="48" height="48" rx="10" fill="#1a1a1a"/>
          <rect x="8" y="10" width="32" height="28" rx="3" fill="#76B900" opacity="0.8"/>
          <rect x="12" y="14" width="24" height="16" rx="2" fill="#1a1a1a"/>
          <rect x="14" y="34" width="8" height="2" rx="1" fill="#76B900" opacity="0.6"/>
          <rect x="26" y="34" width="8" height="2" rx="1" fill="#76B900" opacity="0.6"/>
        </svg>
        <div>
          <span style="font-family: monospace; font-size: 0.9rem; font-weight: 800; color: #1a3d2b; display: block;">lin-hp</span>
          <p style="font-size: 0.7rem; color: #4a5e52; margin: 1px 0 0 0;">AI Compute</p>
        </div>
      </div>
      <p style="font-size: 0.75rem; color: #1e2e22; margin: 0; line-height: 1.5;">Xeon E5-2650 · 32 threads · 31 GB RAM<br>RTX 3060 12 GB · Ubuntu 24.04</p>
    </div>

    <div style="background: white; border-radius: var(--radius-sm); border: 1px solid var(--border); border-top: 4px solid #c8a45a; padding: 14px 16px;">
      <div style="display:flex; align-items:center; gap:10px; margin-bottom:8px;">
        <svg width="28" height="28" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect width="48" height="48" rx="10" fill="#c8a45a"/>
          <rect x="8" y="12" width="32" height="26" rx="3" fill="white" opacity="0.2"/>
          <rect x="12" y="16" width="24" height="4" rx="1" fill="white" opacity="0.8"/>
          <rect x="12" y="22" width="18" height="4" rx="1" fill="white" opacity="0.6"/>
          <rect x="12" y="28" width="22" height="4" rx="1" fill="white" opacity="0.4"/>
          <path d="M36 38 L36 42 L44 42" stroke="white" stroke-width="2" stroke-linecap="round" fill="none" opacity="0.7"/>
        </svg>
        <div>
          <span style="font-family: monospace; font-size: 0.9rem; font-weight: 800; color: #1a3d2b; display: block;">NAS</span>
          <p style="font-size: 0.7rem; color: #4a5e52; margin: 1px 0 0 0;">Shared Storage</p>
        </div>
      </div>
      <p style="font-size: 0.75rem; color: #1e2e22; margin: 0; line-height: 1.5;">Synology · 172.16.99.17<br>1.5 TB video storage · NFS exports</p>
    </div>

  </div>
</div>

## hh-01 — The Eyes of the Greenhouse

The first node is a **Raspberry Pi 3 Model B** tucked inside the greenhouse, running Debian 13 (Trixie) on a 15 GB microSD card. Its sole purpose is image capture. Connected via WiFi on the 172.16.108.x network, it runs without Docker — a deliberate choice to keep its footprint minimal and its uptime reliable.

The camera is a [Raspberry Pi AI Camera](https://www.raspberrypi.com/documentation/accessories/ai-camera.html), a Sony IMX500-based module with an on-board neural processing unit. For the timelapse use case, the AI acceleration goes largely unused — what matters is consistent, high-quality image output on a configurable schedule. Captured frames are written to a local directory that is exported over NFS and mounted by hh-03, meaning the timelapse builder service can access fresh images without any data transfer scripts.

At the time of capture the machine was running cool at 47°C SoC temperature and had been up for over eight days without a restart — a fair reflection of how little it needs to do.

## hh-02 — The Hands

The second Pi 3 Model B carries a very different workload. Where hh-01 only observes, **hh-02 acts**. It is wired into the physical environment via its GPIO pins and runs the Go services responsible for automated watering and sensor reading.

The attached hardware includes:

- **4-channel relay module** — switches 12V circuits to open and close the solenoid valves that control water flow to each plant bed. The relay channels map directly to the GPIO pins driven by the watering Go service.
- **Capacitive soil moisture sensors** — one per plant zone, returning analogue readings that are digitised and published to the Prometheus metrics endpoint. Capacitive sensors were chosen over resistive ones for longevity in a damp soil environment.
- **DS18B20 waterproof temperature probes** — stainless steel, one-wire protocol, reading soil temperature directly at root level rather than ambient air. The 1-Wire protocol means multiple sensors can share a single GPIO pin with unique device addresses.
- **BMP280 breakout** (Pimoroni) — mounted at canopy level, reading ambient air temperature, barometric pressure, and altitude. This provides the greenhouse climate data that appears in the daily sensor snapshot posts.

Docker 29.2.1 is installed on hh-02, which runs a small number of service containers. The machine connects wirelessly on the same 172.16.108.x subnet as hh-01, keeping both sensing nodes on the same WiFi segment.

## hh-03 — The Brain

The orchestration node is a **Raspberry Pi 400** — the keyboard-integrated form factor that packs a Cortex-A72 quad-core processor and 4 GB of RAM into a compact desktop unit. Compared to the 3B nodes it is noticeably more capable: faster CPU, four times the memory, and a 118 GB microSD card with room to grow.

hh-03 is the only Pi connected via wired Ethernet (172.16.106.153), which gives it a stable, low-latency path to the NAS and the wider LAN. This matters because it is the machine that runs the main Docker Compose stack — the RabbitMQ broker, Traefik proxy, blog-poster, herbhub-manager, tts-narrator, video-publisher, and timelapse-builder services all live here.

It mounts two NFS shares from the NAS at 172.16.99.17: one for video output and one for production resources like avatar files and intro clips. It also mounts hh-01's timelapse directory over NFS, giving the timelapse-builder service direct read access to camera frames as if they were local. The result is a clean separation: hh-01 writes images, hh-03 reads and processes them, without either machine needing to coordinate the transfer explicitly.

At the time of capture hh-03 was running at a comfortable 37°C with two active Docker bridge networks visible in the interface list, indicating the core service stack was up.

## lin-hp — The Muscle

Not strictly a Herb Hub node, but providing the compute that makes the AI-driven features possible. **lin-hp** is an x86 tower running Ubuntu 24.04, built around an **Intel Xeon E5-2650** — an eight-core, sixteen-thread server CPU running at 2.0 GHz with 31 GB of ECC RAM. It is the machine that would have been decommissioned from a server rack somewhere before being repurposed for the home lab.

The critical component is the **NVIDIA GeForce RTX 3060** with 12 GB of VRAM. This is what enables real-time avatar video generation via MuseTalk and fast LLM inference via Ollama (currently running Gemma 4). Without the GPU, video narration would be feasible but slow; with it, a two-minute narration video renders in roughly the time it takes to watch it.

lin-hp mounts the same NAS NFS shares as hh-03 so that completed videos land directly on shared storage, accessible to the publisher service running on hh-03 without any additional file transfer step.

## Shared Storage — The NAS

Sitting at 172.16.99.17 is a Synology NAS providing the shared storage layer that ties the cluster together. Two NFS exports are mounted across both hh-03 and lin-hp: one for video output and one for production resources. With 1.5 TB of video data already accumulated and roughly 1.2 TB free, there is comfortable headroom for continued daily recordings.

The NAS is intentionally kept out of the compute path. It stores and serves files; all processing happens on the Pi and x86 nodes. This keeps the NAS simple, reliable, and easy to back up independently.

## How It Fits Together

<div style="margin: 2rem 0; padding: 1.5rem; background: var(--foam); border-radius: var(--radius-md); border: 1px solid var(--border); font-size: 0.82rem; color: var(--ink-soft); line-height: 1.8;">
  <div style="display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 8px; text-align: center;">
    <div style="background: white; border-radius: 8px; padding: 10px; border: 1px solid var(--border);">
      <strong style="color:var(--forest);">hh-01</strong><br>
      <span style="font-size:0.72rem;color:var(--ink-muted);">captures frames<br>exports via NFS</span>
    </div>
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none"><path d="M5 12h14M13 6l6 6-6 6" stroke="var(--leaf)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
    <div style="background: white; border-radius: 8px; padding: 10px; border: 1px solid var(--border);">
      <strong style="color:var(--forest);">hh-03</strong><br>
      <span style="font-size:0.72rem;color:var(--ink-muted);">runs services<br>builds timelapse</span>
    </div>
  </div>
  <div style="display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 8px; text-align: center; margin-top: 10px;">
    <div style="background: white; border-radius: 8px; padding: 10px; border: 1px solid var(--border);">
      <strong style="color:var(--forest);">hh-02</strong><br>
      <span style="font-size:0.72rem;color:var(--ink-muted);">reads sensors<br>controls watering</span>
    </div>
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none"><path d="M5 12h14M13 6l6 6-6 6" stroke="var(--leaf)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
    <div style="background: white; border-radius: 8px; padding: 10px; border: 1px solid var(--border);">
      <strong style="color:var(--forest);">lin-hp</strong><br>
      <span style="font-size:0.72rem;color:var(--ink-muted);">AI inference<br>video generation</span>
    </div>
  </div>
  <div style="text-align: center; margin-top: 10px;">
    <div style="display:inline-block; background: white; border-radius: 8px; padding: 10px 20px; border: 1px solid var(--border);">
      <strong style="color:var(--forest);">NAS</strong> &nbsp;·&nbsp; <span style="font-size:0.72rem;color:var(--ink-muted);">1.5 TB shared video storage · NFS</span>
    </div>
  </div>
</div>

The two Pi 3B nodes handle the physical world at the edge. hh-01 watches and records; hh-02 listens to the soil and responds with water when needed. Both run quietly on WiFi, drawing minimal power, with no display attached.

hh-03 is the coordinator. It receives data and images from the edge nodes, runs the service stack, and publishes finished content. It is the only node with a wired Ethernet connection — a deliberate choice for a machine that handles all Docker networking and NFS traffic.

lin-hp sits slightly apart from the cluster, providing AI compute on demand rather than running continuously in the loop. When a video needs generating or a language model needs querying, requests flow across the LAN to lin-hp and results come back as files on shared NFS storage.

---

The whole thing draws somewhere in the region of 20–30 watts under normal load — less than a single incandescent bulb — which feels appropriate for a greenhouse that is trying to grow things rather than heat a data centre.

![The Herb Hub control board](/assets/images/thehub.png)
</thinking>
