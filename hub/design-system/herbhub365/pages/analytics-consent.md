# Herb Hub 365 Analytics Consent Pattern

## Intent

Provide a first-party, privacy-conscious analytics consent component that is visually aligned with the botanical/editorial design system while enforcing strict prior consent for GA4.

## Product requirements captured

1. **Strict prior consent**
   - No GA script request before explicit accept.
   - No pre-consent `dataLayer` initialization.
   - No pre-consent `gtag` invocation.
   - No Consent Mode default/denied cookieless pings.

2. **Production-only analytics loading**
   - Build-time flag from Liquid controls GA eligibility:
     - `true` only when `jekyll.environment == 'production'` and measurement ID exists.
   - Consent UI remains functional in development for UX testing.
   - Runtime loading is also restricted to `www.herbhub365.com` and `herbhub365.com`, preventing local and Azure preview traffic from polluting production analytics.
   - The Azure Static Web Apps workflow explicitly sets `JEKYLL_ENV=production`.

3. **Versioned first-party preference state**
   - Stored in localStorage key namespace `herbhub365.analytics-consent.v1`.
   - Supported values: `granted`, `denied`.
   - localStorage failures degrade safely to session behavior.

4. **User controls and withdrawal**
   - Equally prominent accept/reject actions.
   - Privacy page link in consent copy.
   - Persistent footer-level “Cookie settings” reopen control.
   - Withdrawal path:
     - set `window['ga-disable-G-CYF38MD43R'] = true`
     - send `gtag('consent', 'update', { analytics_storage: 'denied' })` only if `gtag` already exists
     - best-effort deletion of `_ga` and `_ga_*` first-party cookies for host and herbhub365.com variants.

5. **Accessibility behavior**
   - Non-modal semantic region with labelled heading and description.
   - Keyboard focus moves to panel when opened.
   - Escape closes only when a prior choice exists.
   - `aria-live` status updates announce state changes.
   - `hidden` attribute controls visibility.
   - No focus trap; rest of page remains usable.

## Component structure

- Include file: `_includes/analytics-consent.html`
- Script file: `assets/js/analytics-consent.js`
- Placement: injected near end of `_layouts/default.html` after footer.

## Visual design notes

- Fixed bottom card, compact and editorial.
- Mobile-safe sizing so content remains visible at 375 width.
- Matching pill-button style for parity with existing CTA language.
- Dark mode variant follows existing botanical night palette.
- Reduced-motion mode disables non-essential transitions.
- Scoped selectors avoid regressions in existing landing/archive/about patterns.

## Privacy UX copy principles

- Plain language over legal wording.
- Explain “why” (site improvement) briefly.
- Present equal choice prominence.
- Provide immediate path to policy details and settings changes.
