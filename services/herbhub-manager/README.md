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

### Automatic watering API (operator protected)

- `GET /api/messaging/automatic-watering`
  - Returns:
    ```json
    {
      "config_revision": 1,
      "config": {
        "enabled": false,
        "evaluation_interval_seconds": 300,
        "cooldown_seconds": 21600,
        "max_metric_age_seconds": 900,
        "prometheus_timeout_seconds": 10,
        "message_expiry_seconds": 300,
        "moisture_metric": "herbhub_soil_percent",
        "plant_label": "plant",
        "plants": {
          "basil": {"enabled": true, "threshold_percent": 30, "metric_label_value": "basil"},
          "chilli": {"enabled": true, "threshold_percent": 30, "metric_label_value": "chilli"},
          "oregano": {"enabled": true, "threshold_percent": 30, "metric_label_value": "oregano"}
        }
      },
      "runtime_status": {
        "running": true,
        "faulted": false,
        "fault": "",
        "instance_id": "hostname-or-container-id",
        "next_evaluation_at": "2026-08-11T19:15:00Z"
      },
      "state": {
        "basil": {
          "last_evaluated_at": "2026-08-11T19:10:00Z",
          "last_sample_at": "2026-08-11T19:09:45Z",
          "last_value": 22.4,
          "last_decision": "water_published",
          "last_error": "",
          "cooldown_until": "2026-08-12T01:10:00Z",
          "last_message_id": "..."
        },
        "chilli": {},
        "oregano": {}
      }
    }
    ```
  - Response includes `ETag: "<config_revision>"`.

- `PUT /api/messaging/automatic-watering`
  - Requires `If-Match: "<current revision>"`.
  - Body (strict full replacement):
    ```json
    {
      "config": { "...": "complete config object required" },
      "confirm_enable": false
    }
    ```
  - Enabling from disabled requires `confirm_enable=true`.
  - Statuses:
    - `428` missing `If-Match`
    - `400` invalid `If-Match` or invalid body/config
    - `412` stale revision
    - `503` store faulted/unsafe
    - `200` success (returns updated document + new `ETag`)

### Automatic watering safety/operations

- Decision engine is in Manager; watering service remains a separate consumer.
- **Single-replica invariant:** automatic watering supports exactly one Manager instance writing one shared state volume (`/var/lib/herbhub-manager`). This is an operational lock invariant, not distributed leadership.
- Automation default is **disabled** and persists in `/var/lib/herbhub-manager/automatic-watering.json`.
- Manager publishes only actionable `water`; it never auto-publishes `skip`.
- Physical watering publish requires exactly one observed `watering.queue` consumer. This is an operational safety check only (consumer count is not cryptographic identity).
- Before enablement, verify Prometheus metric identity/freshness semantics:
  - metric name and plant label key/values are correct for your scrape target,
  - sample timestamps reflect source freshness (not only scrape time),
  - one and only one series is returned per fixed plant.
- If state store cannot initialize or faults later, HTTP API remains available and automatic watering is reported faulted/unsafe.

Freshness interpretation details:

- Evaluator uses a single evaluation timestamp per cycle and executes both:
  - raw selector query for moisture value, and
  - `timestamp(selector)` query for source sample Unix time.
- Staleness/future checks are applied to the source sample timestamp from `timestamp(...)`, not the instant-vector envelope timestamp.

Bootstrap environment values (used only for first state creation):

- `AUTOWATERING_STATE_PATH` (default `/var/lib/herbhub-manager/automatic-watering.json`)
- `AUTOWATERING_ENABLED` (strict bool; default `false`)
- `AUTOWATERING_EVALUATION_INTERVAL_SECONDS` (default `300`, range `60..3600`)
- `AUTOWATERING_COOLDOWN_SECONDS` (default `21600`, range `3600..604800`)
- `AUTOWATERING_MAX_METRIC_AGE_SECONDS` (default `900`, range `60..3600`, must be `>= evaluation interval`)
- `AUTOWATERING_PROMETHEUS_TIMEOUT_SECONDS` (default `10`, range `1..30`)
- `AUTOWATERING_MESSAGE_EXPIRY_SECONDS` (default `300`, range `30..300`)
- `AUTOWATERING_MOISTURE_METRIC` (default `herbhub_soil_percent`, Prometheus identifier only)
- `AUTOWATERING_PLANT_LABEL` (default `plant`, Prometheus identifier only)
- Per-plant fixed keys (`basil|chilli|oregano`):
  - `AUTOWATERING_PLANT_<PLANT>_ENABLED`
  - `AUTOWATERING_PLANT_<PLANT>_THRESHOLD_PERCENT` (range `5..80`)
  - `AUTOWATERING_PLANT_<PLANT>_METRIC_LABEL_VALUE` (safe label value only)

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
