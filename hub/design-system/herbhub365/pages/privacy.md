# Herb Hub 365 Privacy Page Pattern

## Intent

Present privacy and analytics consent in a calm, trust-focused editorial format that aligns with About and archive systems while remaining less cinematic and more policy-oriented.

## UX and content requirements

1. **Compact editorial hero**
   - Uses a concise kicker + H1 + short intro + last-updated treatment.
   - Avoids oversized slab headers and avoids full-bleed cinematic framing.

2. **Status cues with iconography**
   - Uses Heroicons (no emojis) for quick interpretation:
     - consent-first analytics
     - transparency and controls

3. **Structured policy cards**
   - Responsive card layout covering:
     - provider and purpose
     - consent and legal basis
     - collected data categories
     - storage and cookies
     - withdrawal and settings controls
     - retention and contact
   - Retains legal/transparency substance from previous prose version.

4. **Prominent settings control**
   - Includes in-page CTA button with `data-analytics-open-settings`.
   - Footer-level Cookie settings control remains available site-wide.

5. **Accessibility and robustness**
   - Heading order remains linear and valid.
   - Focus-visible styles are explicit.
   - Reduced-motion mode removes decorative transitions.
   - No horizontal overflow at 375/768/1024/1440 widths.
   - Hover states do not shift layout.

6. **Dark mode contrast safety**
   - Policy card headings, strong text, and links avoid deep-green-on-deep-green combinations.
   - Explicit high-contrast text values are used in dark mode.

## Implementation scope

- Layout: `_layouts/privacy.html`
- Page: `privacy.markdown`
- Styles: `assets/main.scss`, fully scoped under `.privacy-page`
- No global token remapping required; hero/background token behavior remains unchanged for existing landing/about/category systems.
