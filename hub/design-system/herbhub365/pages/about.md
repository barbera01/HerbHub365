# Herb Hub 365 About Page

## Intent

Deliver a **dedicated About experience** that matches the premium media-first editorial language of landing and archive pages, while preserving the existing About copy’s meaning and most wording.

## Page-specific direction

1. **Dedicated layout, no global static-page changes**
   - Implemented in `_layouts/about.html`.
   - `about.markdown` now uses `layout: about`.
   - Existing `_layouts/page.html` remains untouched for other static pages.

2. **Cinematic media-led hero**
   - Hero image is dynamically extracted from the newest post containing a rendered `<img ... src="...">`.
   - Supports both `src="..."` and `src='...'` extraction styles.
   - Fallback is `/assets/images/thehub.png`.
   - Hero includes descriptive alt text tied to the selected visual post title.

3. **Explicit project framing**
   - Hero copy states this is a personal learning and development project.
   - Explicitly connects real greenhouse work with software, sensors, cloud, observability, automation, and AI.
   - Includes clear CTAs to greenhouse journal (`/herbs/`) and GitHub source repository.
   - Uses inline Heroicons only (no emoji).

4. **Editorial section architecture for About content**
   - Manifesto-style intro section.
   - “What the project explores” rendered as six responsive cards.
   - “Learning in public” as a distinct editorial statement band.
   - “Source code” callout with repository CTA.
   - Wording preserved from original `about.markdown` wherever possible.

5. **Project facts/signals row using real data**
   - `site.posts | size` for journal count.
   - `site.data.live_sensors.updated_at` for sensor snapshot signal (with safe fallback copy).
   - Explicit non-inflated labels for open-source and iterative mode.

## Accessibility and interaction requirements

- Semantic landmarks and heading order:
  - `<article>` root
  - `<header>` hero with `<h1>`
  - sequential `<section>` blocks with `<h2>` then `<h3>` in cards
- Visible `:focus-visible` states for interactive elements.
- Stable 150–300ms transitions without layout-shifting transforms.
- `prefers-reduced-motion` support by disabling transitions.
- No JavaScript dependencies for rendering primary content.

## Styling strategy

- Styles scoped under `.about-page` in `assets/main.scss`.
- No selector broadening that affects generic `.page-content`, post, or archive templates.
- Maintains botanical identity:
  - Lora and Raleway typography
  - forest/sage/cream/clay/gold palette
- Includes dedicated dark mode treatment and mobile breakpoints.

## Responsive targets

- Explicit breakpoints cover and validate intended layouts at:
  - 375
  - 768
  - 1024
  - 1440
- Horizontal overflow is prevented via scoped clipping and responsive grid collapse.
