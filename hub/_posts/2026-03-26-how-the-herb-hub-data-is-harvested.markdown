---
layout: post
title: "How the Herb Hub data is harvested"
date: 2026-03-26 19:17:49 +0000
categories: Platform Update
---

The Herb Hub platform relies on a robust pipeline for collecting sensor information and distributing that telemetry across its infrastructure. At the heart of this process lies an automated mechanism designed to capture snapshots from various sensors, format them into standard JSON payloads, and securely deliver them via a message bus architecture using RabbitMQ. This system ensures that data flows reliably from edge devices to central processing nodes without manual intervention.

The harvesting workflow begins with a dedicated shell script responsible for orchestrating the collection event. Before any data is transmitted, the environment validates its configuration by checking for necessary credentials and ensuring all required scripts are present and executable within their designated directories. This validation step prevents unauthorized access attempts or accidental exposure of sensitive information during routine operations. Once validated, the system initiates a secure connection to the RabbitMQ API endpoint located at herbhub365.com.

The core functionality involves generating a snapshot file containing sensor readings in JSON format. The system then prepares this data for transmission by constructing authentication headers using Base64 encoding of user credentials and password combinations. This encoded information is attached to HTTP requests alongside standard content-type definitions, ensuring that every message sent over the network carries appropriate security context. The payload undergoes a final validation check against strict JSON formatting rules before being queued for delivery, guaranteeing data integrity throughout the transfer process.

Data transmission occurs through a structured interaction with RabbitMQ queues and exchanges configured specifically for sensor telemetry. The system defines routing keys that match queue names to ensure messages reach their intended destinations within specific virtual hosts. Messages are marked as durable and persistent unless explicitly set otherwise, allowing them to survive temporary network interruptions or service restarts without data loss. Each published message includes metadata such as content type specifications and delivery modes that govern how consuming services handle the incoming telemetry streams.

The architecture supports optional persistence of snapshot files after successful publication based on configurable flags within the environment variables. When enabled, copies are retained in designated output paths for audit purposes or offline analysis while maintaining a clean operational state by defaulting to temporary storage locations during active collection cycles. This approach balances data availability with resource management, ensuring that historical records remain accessible without cluttering primary working directories unnecessarily.

This message bus integration forms the backbone of Herb Hub's real-time monitoring capabilities. By leveraging RabbitMQ as an intermediary layer between sensor sources and downstream analytics engines, the platform achieves high throughput and decoupled system design patterns essential for scalable IoT deployments. The combination of rigorous pre-flight checks, secure authentication protocols, and reliable queue management ensures that harvested data reaches its destination accurately and promptly regardless of network volatility or infrastructure changes.

## Pipeline Architecture

<div style="background:#1a1a1a; border-radius:12px; padding:16px; margin:2rem 0; overflow-x:auto;">
<svg width="100%" viewBox="0 0 680 527.15" xmlns="http://www.w3.org/2000/svg">
<defs>
<marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="context-stroke" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
</defs>

<!-- Edge devices tier -->
<text x="340" y="30" text-anchor="middle" opacity="0.6" style="fill:rgb(194, 192, 182);font-family:system-ui,sans-serif;font-size:12px;">Edge devices</text>

<g>
<rect x="40" y="44" width="130" height="56" rx="8" stroke-width="0.5" style="fill:rgb(39, 80, 10);stroke:rgb(151, 196, 89);"/>
<text x="105" y="62" text-anchor="middle" dominant-baseline="central" style="fill:rgb(192, 221, 151);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Soil moisture</text>
<text x="105" y="80" text-anchor="middle" dominant-baseline="central" style="fill:rgb(151, 196, 89);font-family:system-ui,sans-serif;font-size:12px;">ADS1115 ADC</text>
</g>

<g>
<rect x="190" y="44" width="130" height="56" rx="8" stroke-width="0.5" style="fill:rgb(39, 80, 10);stroke:rgb(151, 196, 89);"/>
<text x="255" y="62" text-anchor="middle" dominant-baseline="central" style="fill:rgb(192, 221, 151);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Temperature</text>
<text x="255" y="80" text-anchor="middle" dominant-baseline="central" style="fill:rgb(151, 196, 89);font-family:system-ui,sans-serif;font-size:12px;">DS18B20</text>
</g>

<g>
<rect x="360" y="44" width="130" height="56" rx="8" stroke-width="0.5" style="fill:rgb(39, 80, 10);stroke:rgb(151, 196, 89);"/>
<text x="425" y="62" text-anchor="middle" dominant-baseline="central" style="fill:rgb(192, 221, 151);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Relay control</text>
<text x="425" y="80" text-anchor="middle" dominant-baseline="central" style="fill:rgb(151, 196, 89);font-family:system-ui,sans-serif;font-size:12px;">Waveshare board</text>
</g>

<g>
<rect x="510" y="44" width="130" height="56" rx="8" stroke-width="0.5" style="fill:rgb(39, 80, 10);stroke:rgb(151, 196, 89);"/>
<text x="575" y="62" text-anchor="middle" dominant-baseline="central" style="fill:rgb(192, 221, 151);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Light / humidity</text>
<text x="575" y="80" text-anchor="middle" dominant-baseline="central" style="fill:rgb(151, 196, 89);font-family:system-ui,sans-serif;font-size:12px;">Env sensors</text>
</g>

<!-- Arrows from sensors down -->
<line x1="105" y1="100" x2="105" y2="140" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>
<line x1="255" y1="100" x2="255" y2="140" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>
<line x1="425" y1="100" x2="425" y2="140" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>
<line x1="575" y1="100" x2="575" y2="140" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>

<!-- Collection layer -->
<text x="340" y="138" text-anchor="middle" opacity="0.6" style="fill:rgb(194, 192, 182);font-family:system-ui,sans-serif;font-size:12px;">Collection layer</text>

<g>
<rect x="60" y="150" width="560" height="80" rx="14" stroke-width="0.5" style="fill:rgb(8, 80, 65);stroke:rgb(93, 202, 165);"/>
<text x="340" y="180" text-anchor="middle" dominant-baseline="central" style="fill:rgb(159, 225, 203);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Raspberry Pi 3B — harvest script</text>
<text x="340" y="200" text-anchor="middle" dominant-baseline="central" style="fill:rgb(93, 202, 165);font-family:system-ui,sans-serif;font-size:12px;">Env validation → JSON snapshot → Base64 auth → HTTP publish</text>
</g>

<!-- Arrow to RabbitMQ -->
<line x1="340" y1="230" x2="340" y2="275" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>
<text x="350" y="258" opacity="0.5" style="fill:rgb(194, 192, 182);font-family:system-ui,sans-serif;font-size:12px;">HTTPS</text>

<!-- RabbitMQ broker -->
<text x="340" y="273" text-anchor="middle" opacity="0.6" style="fill:rgb(194, 192, 182);font-family:system-ui,sans-serif;font-size:12px;">Message bus</text>

<g>
<rect x="140" y="285" width="400" height="80" rx="14" stroke-width="0.5" style="fill:rgb(60, 52, 137);stroke:rgb(175, 169, 236);"/>
<text x="340" y="315" text-anchor="middle" dominant-baseline="central" style="fill:rgb(206, 203, 246);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">RabbitMQ @ herbhub365.com</text>
<text x="340" y="335" text-anchor="middle" dominant-baseline="central" style="fill:rgb(175, 169, 236);font-family:system-ui,sans-serif;font-size:12px;">Exchanges → routing keys → durable queues</text>
</g>

<!-- Fan out arrows -->
<line x1="240" y1="365" x2="120" y2="410" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>
<line x1="340" y1="365" x2="340" y2="410" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>
<line x1="440" y1="365" x2="560" y2="410" marker-end="url(#arrow)" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:1.5px;"/>

<!-- Consumers tier -->
<text x="340" y="405" text-anchor="middle" opacity="0.6" style="fill:rgb(194, 192, 182);font-family:system-ui,sans-serif;font-size:12px;">Consumers</text>

<g>
<rect x="40" y="418" width="160" height="56" rx="8" stroke-width="0.5" style="fill:rgb(113, 43, 19);stroke:rgb(240, 153, 123);"/>
<text x="120" y="436" text-anchor="middle" dominant-baseline="central" style="fill:rgb(245, 196, 179);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Blog poster</text>
<text x="120" y="454" text-anchor="middle" dominant-baseline="central" style="fill:rgb(240, 153, 123);font-family:system-ui,sans-serif;font-size:12px;">Ollama LLM posts</text>
</g>

<g>
<rect x="260" y="418" width="160" height="56" rx="8" stroke-width="0.5" style="fill:rgb(113, 43, 19);stroke:rgb(240, 153, 123);"/>
<text x="340" y="436" text-anchor="middle" dominant-baseline="central" style="fill:rgb(245, 196, 179);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Analytics engine</text>
<text x="340" y="454" text-anchor="middle" dominant-baseline="central" style="fill:rgb(240, 153, 123);font-family:system-ui,sans-serif;font-size:12px;">Trend analysis</text>
</g>

<g>
<rect x="480" y="418" width="160" height="56" rx="8" stroke-width="0.5" style="fill:rgb(113, 43, 19);stroke:rgb(240, 153, 123);"/>
<text x="560" y="436" text-anchor="middle" dominant-baseline="central" style="fill:rgb(245, 196, 179);font-family:system-ui,sans-serif;font-size:14px;font-weight:500;">Snapshot archive</text>
<text x="560" y="454" text-anchor="middle" dominant-baseline="central" style="fill:rgb(240, 153, 123);font-family:system-ui,sans-serif;font-size:12px;">Audit / offline</text>
</g>

<!-- Persistence note -->
<line x1="620" y1="474" x2="620" y2="500" style="fill:none;stroke:rgb(156, 154, 146);stroke-width:0.5px;stroke-dasharray:4,3;"/>
<text x="620" y="514" text-anchor="middle" opacity="0.5" style="fill:rgb(194, 192, 182);font-family:system-ui,sans-serif;font-size:12px;">Configurable via env flag</text>
</svg>
</div>


<div class="video-embed">
  <iframe src="https://www.youtube.com/embed/MOcuq9n0Yfc" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe>
</div>