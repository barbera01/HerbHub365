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

### Curated RabbitMQ messaging management (optional)

These settings are **separate** from `RABBITMQ_URL` / `RABBITMQ_QUEUE` used by video publishing.

- `RABBITMQ_MANAGEMENT_ENABLED` (default `false`)
- `RABBITMQ_MANAGEMENT_URL` (default `http://rabbitmq:15672`)
- `RABBITMQ_MANAGEMENT_VHOST` (default `/`)
- `RABBITMQ_MANAGEMENT_USER` (required when enabled)
- `RABBITMQ_MANAGEMENT_PASSWORD` (required when enabled)
- `RABBITMQ_MANAGEMENT_TIMEOUT` (default `10s`)
- `PROMETHEUS_URL` (optional; blank disables summaries)
- `PROMETHEUS_TIMEOUT` (default `10s`)
- `GRAFANA_URL` (optional link returned to client)

Credentials are never returned by API responses.

Prometheus summaries are best-effort only and require RabbitMQ per-object queue metrics
to be scraped (including `queue` + `vhost` labels). If those series are unavailable,
the overview still works and reports metrics unavailable.

### SPA runtime auth variables (in nginx env.js)

- `SPA_AUTH_AUTHORITY`
- `SPA_AUTH_DISABLED` (default `false`, local dev only)
- `SPA_AUTH_CLIENT_ID`
- `SPA_AUTH_REDIRECT_URI`
- `SPA_AUTH_POST_LOGOUT_REDIRECT_URI`
- `SPA_AUTH_API_SCOPE`
- `SPA_AUTH_API_AUDIENCE`
- `SPA_AUTH_REQUIRED_ROLE`

Do **not** bake tenant/client IDs into the built bundle; they are injected at container runtime. The generated `env.js` is written to the `/tmp` tmpfs so nginx can keep a read-only root filesystem.

## Timelapse server-to-server media handoff

- `TIMELAPSE_INTERNAL_URL` is used for narrator fetches of timelapse media.
- API provides `/internal/timelapse/videos/{filename}` for this handoff.
- nginx explicitly denies `/internal/*` so it is not browser-exposed.
- Browser access remains through authenticated `/api/timelapse/videos/*`.

## Messaging API (curated, non-generic)

All messaging routes are protected by workforce auth (`Manager.Operator`) through existing middleware.

- `GET /api/messaging/overview`
  - Returns enablement/broker status, optional `grafana_url`, static catalogue status,
    template metadata, and optional Prometheus queue summaries.
- `POST /api/messaging/catalogues/{id}/provision`
  - Allowlisted IDs only (`watering`, `plant-health`).
  - Non-destructive, idempotent reconcile (create missing only).
  - `404` unknown catalogue, `409` topology drift/conflict, `503` disabled/unavailable.
- `POST /api/messaging/templates/{id}/publish`
  - Body: `{ "payload": <json-object>, "routing_key": "optional", "confirmed": boolean }`
  - Returns: `{ "message_id", "exchange", "routing_key", "routed" }`
  - `404` unknown template, `400` validation error, `409` confirmation required for physical watering, `503` unavailable.

Templates:

- `watering-water`
  - Strict payload schema `{plant, action, value}`; `action` must be `water`.
  - Route derived as `watering.<plant>`.
  - Requires `confirmed=true`.
  - Publishes with RabbitMQ message expiration `300000` ms (5 minutes).
  - Requires curated watering topology `ready` **and** at least one `watering.queue` consumer.
- `watering-skip`
  - Strict payload schema `{plant, action, value}`; `action` must be `skip`.
  - Route derived as `watering.<plant>`.
  - Does not require an active queue consumer.
- `plant-health-json`
  - Any JSON object up to 64 KiB.
  - Routing key allowlist: `plant.health.left|middle|right` (default `left`).
