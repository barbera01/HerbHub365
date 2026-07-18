# Herb Hub 365 Article Listings (Herbs / Metrics / Platform)

## Purpose

Define a shared premium archive system for the three listing pages while preserving each page’s editorial character.

## Shared archive frame

1. Premium header contains:
   - page description
   - matching article count
   - latest publish date
   - compact cross-navigation (Herbs / Metrics / Platform)
2. Statistics are computed from actual category token matches, not title checks.
3. Accessible hierarchy uses semantic sections, list structures, article cards, and heading order.

## Category-specific intent

### Herbs

- Image-led greenhouse journal grid.
- First matching post is featured.
- Post imagery extracted from first rendered `<img src="...">` (double or single quote support).
- Fallbacks:
  1. `/assets/images/logo.png` for featured
  2. decorative CSS botanical illustration for cards with no images
- Excerpts must fall back to stripped body text if excerpt is empty.

### Metrics

- Dense observability archive optimized for scanning.
- Latest snapshot gets an expanded summary card.
- All snapshots are listed in compact rows (no silent cap).
- Rows surface:
  - date
  - title
  - `prometheus_chart_exports` count
  - chart/data badges using Heroicons

### Platform Architecture

- Spacious technical editorial card layout.
- Image priority:
  1. explicit `post.image`
  2. first rendered image from content
  3. `/assets/images/thehub.png`
- Surfaces technical category cues and richer excerpt fallback.

## Accessibility and interaction

- 4.5:1 contrast minimum in light and dark themes.
- Focus-visible states are explicit on navigation and primary links.
- Hover transitions are stable and subtle (roughly 150–300ms).
- No layout-shifting hover transforms.
- No emoji icons.
- `prefers-reduced-motion` supported.

## Responsive behavior

- Validated target widths: 375 / 768 / 1024 / 1440.
- No horizontal overflow.
- All matched posts remain visible without JavaScript.

## Styling scope

- Archive-specific styles are strictly scoped under `.category-archive`.
- Avoid broad selectors that alter individual article page templates.
