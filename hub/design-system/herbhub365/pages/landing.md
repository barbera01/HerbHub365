# Herb Hub 365 Landing Page (Blog Home)

## Intent

Create a **media-first editorial landing page** that frames Herb Hub 365 as a personal, evolving greenhouse lab where horticulture, telemetry, and AI experimentation converge.

## Page-specific direction

1. **Cinematic, image-led hero**
   - Uses the latest available greenhouse image from post content (`](` extraction pattern), preferring Azure blob-hosted images.
   - Falls back to `/assets/images/thehub.png` when no extractable image is present.
   - Avoids video dependency for hero reliability.

2. **Narrative framing of the project**
   - Hero copy positions the site as a personal learning/growing lab.
   - Includes explicit CTAs to key pathways: Herbs, Platform Architecture, Metrics.
   - Adds a compact “inside the project” three-pillar band (Grow / Sense / Learn).

3. **Visual journal as editorial grid**
   - First card is featured and larger to create clear hierarchy.
   - Card imagery is derived from first markdown image in post content (same extraction strategy).
   - Non-visual posts are skipped for this section to prevent empty image cards.

4. **Category pathways, not archive blocks**
   - Replaces heavy archive-style cards with concise editorial pathways.
   - Preserves existing links and category intent:
     - `/herbs/`
     - `/platform-architecture/`
     - `/metrics/`

5. **Live sensor section retained and relocated lower**
   - Existing sensor data functionality remains intact.
   - Section appears after storytelling and pathways, as a supporting real-time module.

## Accessibility and interaction requirements

- Use semantic sectioning and list/article structure.
- Ensure visible `:focus-visible` treatment across interactive controls.
- Keep hover transitions stable and subtle (target 150–300ms).
- Avoid layout-shifting transforms for cards/buttons.
- Respect `prefers-reduced-motion` by disabling motion-heavy transitions and scale effects.
- Maintain no-horizontal-scroll behavior at 375px viewport width.

## Styling strategy

- Append landing-specific styles in `assets/main.scss` under scoped namespace `.landing-home`.
- Avoid broad selector overrides that could regress post/category pages.
- Preserve existing botanical identity:
  - Typography: Lora + Raleway
  - Palette: forest / sage / cream / clay / gold
- Keep dark mode compatibility with targeted `prefers-color-scheme: dark` overrides.

## Notes

- Heroicons are used consistently for iconography.
- No emoji are used.
- Inline style attributes were removed from landing markup in favor of SCSS classes.
