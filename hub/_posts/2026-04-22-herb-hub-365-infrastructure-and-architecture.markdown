---
layout: post
title: "Herb Hub 365 — Infrastructure and Architecture"
date: 2026-04-22 16:00:00 +0000
categories: Platform Update
---

<script src="https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js"></script>
<script>
  mermaid.initialize({
    startOnLoad: true,
    theme: 'default',
    flowchart: { useMaxWidth: true, htmlLabels: true },
    sequence: { useMaxWidth: true, showSequenceNumbers: false },
    themeVariables: {
      primaryColor: '#dbeafe',
      primaryBorderColor: '#3b82f6',
      secondaryColor: '#fef9c3',
      tertiaryColor: '#dcfce7',
      lineColor: '#4b5563',
      fontFamily: 'Raleway, -apple-system, sans-serif',
    }
  });
</script>

This post is a technical deep-dive into how Herb Hub 365 is wired together — the services, queues, data flows, and external integrations that run beneath the daily greenhouse updates. The diagrams below are generated directly from the live architecture definition and reflect the current state of the platform.

<div style="display:flex;flex-wrap:wrap;gap:10px;margin:1.5rem 0;padding:1rem 1.25rem;background:white;border-radius:var(--radius-sm);border:1px solid var(--border);font-size:0.8rem;">
  <span style="display:inline-flex;align-items:center;gap:6px;color:#2d3d32;font-weight:600;"><span style="width:14px;height:14px;border-radius:3px;background:#dbeafe;border:2px solid #3b82f6;display:inline-block;flex-shrink:0;"></span>Scheduled / daemon service</span>
  <span style="display:inline-flex;align-items:center;gap:6px;color:#2d3d32;font-weight:600;"><span style="width:14px;height:14px;border-radius:3px;background:#dcfce7;border:2px solid #16a34a;display:inline-block;flex-shrink:0;"></span>Management / interactive</span>
  <span style="display:inline-flex;align-items:center;gap:6px;color:#2d3d32;font-weight:600;"><span style="width:14px;height:14px;border-radius:3px;background:#fef9c3;border:2px solid #ca8a04;display:inline-block;flex-shrink:0;"></span>Message bus / queue</span>
  <span style="display:inline-flex;align-items:center;gap:6px;color:#2d3d32;font-weight:600;"><span style="width:14px;height:14px;border-radius:3px;background:#fee2e2;border:2px solid #dc2626;display:inline-block;flex-shrink:0;"></span>Publishing / upload</span>
  <span style="display:inline-flex;align-items:center;gap:6px;color:#2d3d32;font-weight:600;"><span style="width:14px;height:14px;border-radius:3px;background:#fce7f3;border:2px solid #db2777;display:inline-block;flex-shrink:0;"></span>External API / cloud</span>
  <span style="display:inline-flex;align-items:center;gap:6px;color:#2d3d32;font-weight:600;"><span style="width:14px;height:14px;border-radius:3px;background:#f3f4f6;border:2px solid #6b7280;display:inline-block;flex-shrink:0;"></span>Infrastructure</span>
</div>

## Full System Architecture

The complete platform spans IoT edge devices, eight Go microservices, a RabbitMQ message broker, shared file storage, and a small number of external APIs. Data flows left to right: physical sensors and the timelapse camera feed into the service layer, which coordinates content generation, video production, and publishing via asynchronous queues.

<div style="margin:1.5rem 0;background:white;border-radius:12px;padding:1.25rem 1.5rem;overflow-x:auto;border:1px solid #dde8e2;"><div class="mermaid">
flowchart LR
    subgraph IOT["🌱 IoT &amp; External Sources"]
        direction TB
        SENSORS["Sensors\nhh-02:9100\n(node_exporter)"]
        CAMERA["Timelapse\nCamera"]
        BROWSER["User / Browser"]
    end

    subgraph SCHED["⏰ Scheduled Services"]
        direction TB
        BLOGPOSTER["blog-poster\ncron 00:05 UTC\n+ prom-post 23:00"]
        TTS["tts-narrator\ncron 00:10 UTC"]
        VIDNAR["video-narrator\ndaemon / server\n:8090"]
        TLAPSE["timelapse-builder\n:8082"]
        WATERING["watering\n5 min poll"]
    end

    subgraph MGMT["🎛️ Management"]
        MANAGER["herbhub-manager\n:8080\nWeb UI + REST API"]
    end

    subgraph MQ["📨 RabbitMQ — :5672 AMQP · :15672 Mgmt API"]
        direction TB
        Q1["⊟ sensor.snapshots"]
        Q2["⊟ video.produced"]
        QDLQ["⊟ video.produced.dlq"]
        Q3["⊟ watering.queue"]
    end

    subgraph DATA["🗄️ Data Stores"]
        direction TB
        JEKYLL[("Jekyll Repo\n_posts/\nassets/")]
        VIDOUT[("Video Output\n.mp4 + .json")]
        AUDIOOUT[("Audio Output\n.mp3")]
    end

    subgraph PUBLISH["📤 Publishing"]
        VIDPUB["video-publisher\nYouTube uploader"]
    end

    subgraph APIS["☁️ External APIs"]
        direction TB
        LLM["Ollama / LLM\nollama.la.home-cloud.uk"]
        KOKORO["Kokoro TTS\nkokoro-api.lab.home-cloud.uk"]
        MUSETALK["MuseTalk\n[ai-host]:8011"]
        YOUTUBE["YouTube API\ngoogleapis.com"]
        GITHUB["GitHub\nJekyll Repo"]
        PROM["Prometheus\nprometheus.home-cloud.uk"]
    end

    subgraph INFRA["🔧 Infrastructure"]
        direction TB
        TRAEFIK["Traefik\nReverse Proxy"]
        CRONICLE["Cronicle\nScheduler :3012"]
    end

    SENSORS -->|"AMQP publish"| Q1
    CAMERA -->|"images mount"| TLAPSE
    BROWSER -->|"HTTPS"| TRAEFIK
    TRAEFIK -->|"HTTP"| MANAGER

    Q1 -->|"AMQP consume"| BLOGPOSTER
    BLOGPOSTER -->|"HTTP"| LLM
    BLOGPOSTER -->|"writes posts"| JEKYLL
    BLOGPOSTER -->|"git push"| GITHUB

    JEKYLL -->|"reads posts"| TTS
    TTS -->|"HTTPS POST"| KOKORO
    TTS -->|"writes MP3"| AUDIOOUT
    TTS -->|"git push"| GITHUB

    JEKYLL -->|"reads posts"| VIDNAR
    VIDNAR -->|"HTTP"| MUSETALK
    VIDNAR -->|"writes MP4"| VIDOUT
    VIDNAR -->|"AMQP publish"| Q2

    MANAGER -->|"HTTPS"| VIDNAR
    MANAGER -->|"HTTP"| TLAPSE
    MANAGER -->|"HTTP"| LLM
    MANAGER -->|"HTTP :15672\nmgmt API"| Q2

    Q2 -->|"AMQP consume"| VIDPUB
    VIDPUB -->|"HTTPS OAuth2"| YOUTUBE
    VIDPUB -->|"updates embed\ngit push"| GITHUB
    VIDPUB -->|"on failure"| QDLQ

    PROM -->|"HTTP scrape"| WATERING
    WATERING -->|"AMQP publish"| Q3

    style VIDPUB fill:#fee2e2,stroke:#dc2626,color:#7f1d1d
    style MANAGER fill:#dcfce7,stroke:#16a34a,color:#14532d
    style Q2 fill:#fef9c3,stroke:#ca8a04
    style BLOGPOSTER fill:#dbeafe,stroke:#3b82f6
    style TTS fill:#dbeafe,stroke:#3b82f6
    style VIDNAR fill:#dbeafe,stroke:#3b82f6
</div></div>

## Video Content Pipeline

Every narrated video on this site follows a deterministic pipeline that starts with a sensor reading and ends with a YouTube embed injected into a blog post. The diagram below traces the full end-to-end flow, including the two paths by which video generation can be triggered: automatically by the daemon or manually via the manager web UI.

<div style="margin:1.5rem 0;background:white;border-radius:12px;padding:1.25rem 1.5rem;overflow-x:auto;border:1px solid #dde8e2;"><div class="mermaid">
flowchart TD
    A["📡 IoT Sensors\nsoil / environment data"] -->|"AMQP → sensor.snapshots"| B

    B["blog-poster\ncron 00:05 UTC"] -->|"HTTP POST"| C["Ollama / LLM\nContent generation"]
    C -->|"Returns generated content"| B
    B -->|"Writes markdown"| D[("Jekyll _posts/\nYYYY-MM-DD-slug.md")]
    B -->|"git push"| GH["GitHub\nherbhub365.com"]

    D -->|"reads"| E["tts-narrator\ncron 00:10 UTC"]
    E -->|"HTTPS POST text"| F["Kokoro TTS API\nMP3 generation"]
    F -->|"audio stream"| E
    E -->|"writes .mp3"| AU[("assets/audio/blog/\nYYYY-MM-DD-slug.mp3")]
    AU -->|"audio_url in front matter\ngit push"| GH

    D -->|"reads"| G

    subgraph VIDGEN["Video Generation — two paths"]
        G["video-narrator\n:8090 daemon/server"]
        GM["herbhub-manager\n:8080 POST /api/generate"]
        GM -->|"HTTPS with post text"| G
    end

    G -->|"HTTP TTS + MuseTalk"| MT["MuseTalk API\n[ai-host]:8011\nAvatar video generation"]
    MT -->|"MP4 stream"| G
    G -->|"writes"| VO[("Video Output\nYYYY-MM-DD-slug.mp4")]
    G -->|"AMQP publish\nvideo.produced"| MQ

    VO -->|"file path in message"| MQ["RabbitMQ\nvideo.produced queue"]

    MQ -->|"AMQP consume"| VP["video-publisher"]
    VP -->|"HTTPS OAuth2\nupload MP4"| YT["YouTube API\ngoogleapis.com"]
    YT -->|"videoId"| VP
    VP -->|"injects iframe embed\ngit push"| GH
    VP -->|"deletes MP4,\nwrites .json marker"| VO

    style GM fill:#dcfce7,stroke:#16a34a
    style VP fill:#fee2e2,stroke:#dc2626
    style MQ fill:#fef9c3,stroke:#ca8a04
</div></div>

## Manual YouTube Publish

In addition to the fully automated pipeline, videos can be published manually from the herbhub-manager web UI. Rather than adding a separate publish endpoint to video-publisher, the manager queues the message directly via the RabbitMQ management HTTP API. The video-publisher consumer picks it up from the same `video.produced` queue and handles the upload identically to the automated path.

<div style="margin:1.5rem 0;background:white;border-radius:12px;padding:1.25rem 1.5rem;overflow-x:auto;border:1px solid #dde8e2;"><div class="mermaid">
sequenceDiagram
    actor User
    participant M as herbhub-manager<br/>:8080
    participant R as RabbitMQ Mgmt API<br/>rabbitmq:15672
    participant Q as video.produced<br/>queue
    participant VP as video-publisher
    participant YT as YouTube API
    participant GH as GitHub

    User->>M: POST /api/publish {slug}
    M->>M: Resolve post → find .mp4 in output dir
    M->>R: POST /api/exchanges/%2F/amq.default/publish<br/>{slug, date, output_file, status:"completed"}
    R->>Q: Route message (delivery_mode:2 persistent)
    R-->>M: {routed: true}
    M-->>User: 202 Accepted {status:"queued"}

    Note over VP,Q: video-publisher consumer picks up message
    VP->>Q: AMQP consume
    Q-->>VP: {slug, date, output_file}
    VP->>VP: Load post metadata (title, tags, excerpt)
    VP->>YT: Upload MP4 (HTTPS OAuth2)
    YT-->>VP: videoId
    VP->>VP: Write .json marker with youtube_url
    VP->>GH: Inject iframe embed, git push
    VP->>VP: Delete local .mp4

    Note over User,M: Posts page badge updates to "Published" on next refresh
</div></div>

## Timelapse Pipeline

Timelapse videos follow a slightly different path. The timelapse-builder service stitches raw camera frames into an MP4 independently of the blog pipeline. When a timelapse is ready to be narrated and published, herbhub-manager triggers video-narrator directly with the timelapse file and a narration script, then the same video production and publishing path handles the rest.

<div style="margin:1.5rem 0;background:white;border-radius:12px;padding:1.25rem 1.5rem;overflow-x:auto;border:1px solid #dde8e2;"><div class="mermaid">
flowchart LR
    CAM["📷 Timelapse Camera\n/home/andy/Pictures/timelapse"] -->|"images mount\n/input"| TB

    TB["timelapse-builder\n:8082"] -->|"POST /api/build"| TB
    TB -->|"ffmpeg stitch"| TVO[("Timelapse .mp4\n/output/")]

    MANAGER["herbhub-manager\n:8080"] -->|"POST /api/timelapse/build"| TB
    MANAGER -->|"POST /api/timelapse/publish\n{timelapse_file, tts_text, title}"| VN

    TB -->|"GET /api/timelapse/videos/{file}"| MANAGER

    VN["video-narrator\n:8090\nNarrate timelapse"] -->|"TTS + MuseTalk"| VN
    VN -->|"writes narrated MP4"| VO2[("Video Output\n.mp4")]
    VN -->|"AMQP publish\nvideo.produced"| MQ2["RabbitMQ\nvideo.produced"]

    MQ2 -->|"AMQP consume"| VP2["video-publisher"]
    VP2 -->|"HTTPS OAuth2"| YT2["YouTube API"]
    VP2 -->|"git push embed"| GH2["GitHub"]

    style MANAGER fill:#dcfce7,stroke:#16a34a
    style VP2 fill:#fee2e2,stroke:#dc2626
    style MQ2 fill:#fef9c3,stroke:#ca8a04
</div></div>

## Watering Automation

The watering subsystem is the most self-contained part of the platform. A Go service on hh-02 polls Prometheus every five minutes to read soil moisture metrics exported by node_exporter. When any zone drops below threshold it publishes a watering event to RabbitMQ and triggers the GPIO relay directly to open the corresponding valve.

<div style="margin:1.5rem 0;background:white;border-radius:12px;padding:1.25rem 1.5rem;overflow-x:auto;border:1px solid #dde8e2;"><div class="mermaid">
flowchart LR
    NE["node_exporter\nhh-02:9100"] -->|"HTTP metrics"| W
    PROM["Prometheus\nprometheus.home-cloud.uk"] -->|"also scrapes"| NE

    W["watering service\n5 min poll"] -->|"compare to threshold\n(default 40%)"| W
    W -->|"below threshold\nAMQP publish"| Q3["⊟ watering.queue"]
    W -->|"GPIO control"| GPIO["GPIO\nWatering Valve"]

    style W fill:#dbeafe,stroke:#3b82f6
</div></div>

## Service Reference

<div style="overflow-x:auto;margin:1.5rem 0;background:white;border-radius:12px;border:1px solid #dde8e2;">
<table style="width:100%;border-collapse:collapse;font-size:0.82rem;">
  <thead>
    <tr style="background:#edf5f0;">
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Service</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Port</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Mode</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Consumes</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Produces</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">External APIs</th>
    </tr>
  </thead>
  <tbody>
    <tr style="border-bottom:1px solid #dde8e2;background:white;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">llm-service</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:8080</span></td>
      <td style="padding:8px 12px;">HTTP server</td>
      <td style="padding:8px 12px;">HTTP from blog-poster, herbhub-manager</td>
      <td style="padding:8px 12px;">Generated text responses</td>
      <td style="padding:8px 12px;">Ollama</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:#f0f7f2;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">blog-poster</td>
      <td style="padding:8px 12px;">—</td>
      <td style="padding:8px 12px;">cron 00:05 + 23:00</td>
      <td style="padding:8px 12px;">RabbitMQ <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">sensor.snapshots</span></td>
      <td style="padding:8px 12px;">Jekyll posts, git push</td>
      <td style="padding:8px 12px;">llm-service, GitHub, Prometheus</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:white;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">tts-narrator</td>
      <td style="padding:8px 12px;">—</td>
      <td style="padding:8px 12px;">cron 00:10</td>
      <td style="padding:8px 12px;">Jekyll <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">_posts/</span></td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">assets/audio/blog/*.mp3</span>, git push</td>
      <td style="padding:8px 12px;">Kokoro TTS</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:#f0f7f2;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">video-narrator</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:8090</span></td>
      <td style="padding:8px 12px;">HTTP server + daemon</td>
      <td style="padding:8px 12px;">Jekyll posts, HTTP from herbhub-manager</td>
      <td style="padding:8px 12px;">Video Output <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">.mp4</span>, AMQP → <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">video.produced</span></td>
      <td style="padding:8px 12px;">MuseTalk, Kokoro TTS</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:white;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">herbhub-manager</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:8080</span></td>
      <td style="padding:8px 12px;">HTTP server + Web UI</td>
      <td style="padding:8px 12px;">Jekyll posts, Video Output</td>
      <td style="padding:8px 12px;">HTTP to services, AMQP via RabbitMQ mgmt API → <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">video.produced</span></td>
      <td style="padding:8px 12px;">video-narrator, timelapse-builder, llm-service</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:#f0f7f2;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">video-publisher</td>
      <td style="padding:8px 12px;">—</td>
      <td style="padding:8px 12px;">AMQP consumer</td>
      <td style="padding:8px 12px;">RabbitMQ <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">video.produced</span></td>
      <td style="padding:8px 12px;">YouTube upload, Jekyll embed git push, <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">.json</span> marker, DLQ on failure</td>
      <td style="padding:8px 12px;">YouTube API, GitHub</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:white;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">timelapse-builder</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:8082</span></td>
      <td style="padding:8px 12px;">HTTP server</td>
      <td style="padding:8px 12px;">Image mount <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">/input</span>, HTTP from herbhub-manager</td>
      <td style="padding:8px 12px;">Timelapse <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">.mp4</span> in <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">/output</span></td>
      <td style="padding:8px 12px;">ffmpeg (local)</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:#f0f7f2;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">watering</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:8787</span> health</td>
      <td style="padding:8px 12px;">5 min poll</td>
      <td style="padding:8px 12px;">Prometheus metrics (hh-02:9100)</td>
      <td style="padding:8px 12px;">AMQP → <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">watering.queue</span>, GPIO valve</td>
      <td style="padding:8px 12px;">Prometheus, node_exporter</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:white;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">RabbitMQ</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:5672 / :15672</span></td>
      <td style="padding:8px 12px;">Infrastructure</td>
      <td colspan="3" style="padding:8px 12px;">Queues: <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">sensor.snapshots</span> · <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">video.produced</span> · <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">video.produced.dlq</span> · <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">watering.queue</span></td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:#f0f7f2;">
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">Traefik</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:80 / :443</span></td>
      <td style="padding:8px 12px;">Reverse proxy</td>
      <td colspan="3" style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">manager.herbhub365.com</span> → herbhub-manager · <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">rabbit.herbhub365.com</span> → RabbitMQ :15672 · <span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">scheduler.herbhub365.com</span> → Cronicle :3012</td>
    </tr>
    <tr>
      <td style="padding:8px 12px;font-weight:600;color:#1a3d2b;">Cronicle</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">:3012</span></td>
      <td style="padding:8px 12px;">Job scheduler</td>
      <td colspan="3" style="padding:8px 12px;">Manages scheduled tasks with web UI</td>
    </tr>
  </tbody>
</table>
</div>

## RabbitMQ Queue Reference

<div style="overflow-x:auto;margin:1.5rem 0;background:white;border-radius:12px;border:1px solid #dde8e2;">
<table style="width:100%;border-collapse:collapse;font-size:0.82rem;">
  <thead>
    <tr style="background:#edf5f0;">
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Queue</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Producer(s)</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Consumer(s)</th>
      <th style="padding:9px 12px;text-align:left;font-weight:700;border-bottom:2px solid #c8ddd0;color:#1a3d2b;">Message shape</th>
    </tr>
  </thead>
  <tbody>
    <tr style="border-bottom:1px solid #dde8e2;background:white;">
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">sensor.snapshots</span></td>
      <td style="padding:8px 12px;">IoT devices / sensors</td>
      <td style="padding:8px 12px;">blog-poster</td>
      <td style="padding:8px 12px;">Sensor snapshot JSON</td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:#f0f7f2;">
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">video.produced</span></td>
      <td style="padding:8px 12px;">video-narrator (daemon)<br>herbhub-manager (via mgmt API)</td>
      <td style="padding:8px 12px;">video-publisher</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">{ slug, date, output_file, status, timestamp }</span></td>
    </tr>
    <tr style="border-bottom:1px solid #dde8e2;background:white;">
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">video.produced.dlq</span></td>
      <td style="padding:8px 12px;">video-publisher (on failure)</td>
      <td style="padding:8px 12px;">Manual inspection</td>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">{ error, timestamp, original }</span></td>
    </tr>
    <tr>
      <td style="padding:8px 12px;"><span style="font-family:'SF Mono',Menlo,monospace;background:#edf5f0!important;color:#1a3d2b!important;padding:2px 6px;border-radius:3px;font-size:0.78em;font-weight:600;">watering.queue</span></td>
      <td style="padding:8px 12px;">watering service</td>
      <td style="padding:8px 12px;">—</td>
      <td style="padding:8px 12px;">Watering event JSON</td>
    </tr>
  </tbody>
</table>
</div>

---

The architecture is intentionally minimal at each boundary — services communicate via HTTP or AMQP rather than shared databases, each service owns its own data path, and the message broker provides the only coupling between the content pipeline and the publishing layer. This keeps any single service replaceable without cascading changes across the platform.
