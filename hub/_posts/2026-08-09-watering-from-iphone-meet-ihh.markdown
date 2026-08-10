---
layout: post
title: "Watering from the iPhone: Meet iHH"
date: 2026-08-09 08:00:00 +0000
categories: Platform Update
image: /assets/images/ihh-watering-app-overview.svg
image_alt: "Preview of the iHH iPhone watering dashboard with connected pump controls"
---

Herb Hub 365 now has a native iPhone companion: **iHH**, a focused SwiftUI app for interacting with the pumps connected to our watering API. It puts the controls needed around the greenhouse into a pocket-sized dashboard, without trying to turn the phone into another full operations console.

The app is designed for the practical moments when standing next to the herbs matters more than sitting at a terminal: checking whether a pump is active, delivering a measured pulse of water, running several pumps together, or stopping every channel quickly.

![The main iHH irrigation dashboard showing connection status, pump controls and a multi-pump action](/assets/images/ihh-watering-app-overview.svg)
<div class="video-embed">
  <iframe src="https://www.youtube.com/embed/9KgJ3sq-eNQ" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe>
</div>


<p style="margin-top:-0.8rem;color:var(--ink-muted);font-size:0.82rem;text-align:center;">The main irrigation dashboard, based on the current SwiftUI interface.</p>

## A live view of the pumps

iHH starts by requesting the current state from the API. The dashboard builds its pump cards dynamically from that response, so it is not tied to a hard-coded list of herbs or GPIO channels. Each card shows the channel name, GPIO pin, current on/off state and whether a timed job is active.

From the same card we can:

- switch an individual pump on or off;
- run a timed pulse, with a default of 30 seconds;
- see the refreshed state after a command completes; and
- pull down at any time to request the latest status.

Durations are capped at ten minutes in the app. While a watering command is in progress, the other mutation controls are disabled to reduce the chance of overlapping actions.

## More than one pump

Single-channel control is useful for spot watering, but the greenhouse also needs coordinated actions. **Multi Pump** lets us select several channels and pulse them for the same duration. **Cycle** works through the available pumps using a total run time and an on-time for each pump.

There are also confirmed **All Off** and **All On** actions. The most important of these is All Off: a clear, immediate control for stopping the complete watering setup when something does not look right.

![Illustrated app screen previews showing cycle, emergency and API settings](/assets/images/ihh-watering-app-controls.svg)

<p style="margin-top:-0.8rem;color:var(--ink-muted);font-size:0.82rem;text-align:center;">App screen previews based on the current SwiftUI interface.</p>

## How the app reaches the greenhouse

The app talks directly to the existing Go watering API. A status refresh uses `GET /status`; pump changes call the relay, pulse, combination, cycle or all-channel endpoints; and every successful change is followed by another status request so the screen reflects the controller again.

The Settings tab keeps this intentionally simple. It stores an editable API base URL, defaults to `http://hh-02:8181`, validates HTTP or HTTPS addresses and provides a connection test through `/healthz`.

<div style="margin:1.75rem 0;padding:1.35rem 1.5rem;background:var(--foam);border:1px solid var(--border);border-left:4px solid var(--gold);border-radius:0 var(--radius-sm) var(--radius-sm) 0;">
  <strong style="display:block;margin-bottom:0.35rem;">Current network scope</strong>
  <span style="color:var(--ink-soft);">iHH is currently a local-network tool. The iPhone must be able to reach the watering API and resolve the greenhouse host. The present API integration does not include user authentication, so it is not intended to be exposed directly to the public internet.</span>
</div>

## A familiar Herb Hub interface

The app carries the same botanical design language as the public site: forest and leaf greens, cream surfaces, warm gold accents and clear rounded controls. It uses native iOS serif and rounded system fonts, keeping the interface consistent without relying on remote font downloads.

iHH is deliberately small in scope today. It does not replace automatic watering, sensor monitoring or the wider Herb Hub Manager. Instead, it adds a direct human control surface to the watering loop: **iPhone → Go API → relay channel → pump**.

That makes it a useful companion for testing, maintenance and hands-on care—and another step toward making the greenhouse platform easier to operate wherever the herbs are.
