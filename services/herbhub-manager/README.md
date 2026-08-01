# Herb Hub Manager

Herb Hub Manager is now split into:

- `herbhub-manager-api` (Go HTTP API, internal only)
- `herbhub-manager-client` (Vue 3 + TypeScript SPA served by nginx-unprivileged)

## Local development

### API

Run the API with secure defaults:

```bash
go run ./cmd/herbhub-manager
```

For local-only testing without Entra tokens:

```bash
AUTH_DISABLED=true go run ./cmd/herbhub-manager
```

> `AUTH_DISABLED` is for local development only.

### Client

```bash
cd apps/client
npm install
npm run dev
```

Vite proxies `/api` and `/healthz` to `http://localhost:8080`.

## Workforce Entra requirements

You must provision:

1. **Workforce API app registration** (Go API audience)
   - Expose API Application ID URI (used for `AUTH_AUDIENCE` / client scope)
   - Define app role: `Manager.Operator`
2. **Workforce SPA app registration** (Vue client)
   - Redirect URI: `https://manager.herbhub365.com/`
   - Post logout URI: `https://manager.herbhub365.com/`
   - Permission to API scope (delegated)
3. Assign workforce users/groups to `Manager.Operator` role on API app.

## Runtime variables

### API auth variables (fail closed by default)

- `AUTH_DISABLED` (default `false`)
- `AUTH_ISSUER_URL` (required unless `AUTH_DISABLED=true`)
- `AUTH_DISCOVERY_URL` (optional override of OIDC discovery document URL)
- `AUTH_AUDIENCE` (required unless `AUTH_DISABLED=true`)
- `AUTH_REQUIRED_ROLE` (default `Manager.Operator`)
- `AUTH_JWKS_MIN_REFRESH` (default `1m`)
- `AUTH_HTTP_TIMEOUT` (default `10s`)

### Optional CORS (development only)

- `ALLOWED_ORIGIN` (e.g. `http://localhost:5173`)

### SPA runtime auth variables (in nginx env.js)

- `SPA_AUTH_AUTHORITY`
- `SPA_AUTH_DISABLED` (default `false`, local dev only)
- `SPA_AUTH_CLIENT_ID`
- `SPA_AUTH_REDIRECT_URI`
- `SPA_AUTH_POST_LOGOUT_REDIRECT_URI`
- `SPA_AUTH_API_SCOPE`
- `SPA_AUTH_API_AUDIENCE`
- `SPA_AUTH_REQUIRED_ROLE`

Do **not** bake tenant/client IDs into the built bundle; they are injected at container runtime.

## Timelapse server-to-server media handoff

- `TIMELAPSE_INTERNAL_URL` is used for narrator fetches of timelapse media.
- API provides `/internal/timelapse/videos/{filename}` for this handoff.
- nginx explicitly denies `/internal/*` so it is not browser-exposed.
- Browser access remains through authenticated `/api/timelapse/videos/*`.
