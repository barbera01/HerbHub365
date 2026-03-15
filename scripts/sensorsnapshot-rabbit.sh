#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ENV_FILE="${RABBITMQ_ENV_FILE:-$SCRIPT_DIR/sensorsnapshot-rabbit.env}"

if [ -f "$ENV_FILE" ]; then
    set -a
    . "$ENV_FILE"
    set +a
fi

SNAPSHOT_SCRIPT="$SCRIPT_DIR/sensorsnapshot.sh"
RABBITMQ_API_URL="${RABBITMQ_API_URL:-https://rabbit.herbhub365.com/api}"
RABBITMQ_USER="${RABBITMQ_USER:-}"
RABBITMQ_PASS="${RABBITMQ_PASS:-}"
RABBITMQ_VHOST="${RABBITMQ_VHOST:-/}"
RABBITMQ_QUEUE="${RABBITMQ_QUEUE:-sensor.snapshots}"
RABBITMQ_ROUTING_KEY="${RABBITMQ_ROUTING_KEY:-$RABBITMQ_QUEUE}"
RABBITMQ_DURABLE="${RABBITMQ_DURABLE:-true}"
RABBITMQ_AUTO_DELETE="${RABBITMQ_AUTO_DELETE:-false}"
RABBITMQ_DELIVERY_MODE="${RABBITMQ_DELIVERY_MODE:-2}"
KEEP_SNAPSHOT_JSON="${KEEP_SNAPSHOT_JSON:-false}"
SNAPSHOT_OUTPUT_PATH="${SNAPSHOT_OUTPUT_PATH:-$PWD/snapshot.json}"

if [ ! -x "$SNAPSHOT_SCRIPT" ]; then
    echo "Snapshot script not found or not executable: $SNAPSHOT_SCRIPT" >&2
    exit 1
fi

if [ -z "$RABBITMQ_USER" ] || [ -z "$RABBITMQ_PASS" ]; then
    echo "Set RABBITMQ_USER and RABBITMQ_PASS before publishing." >&2
    exit 1
fi

tmpdir=$(mktemp -d)
cleanup() {
    rm -rf "$tmpdir"
}
trap cleanup EXIT

(
    cd "$tmpdir"
    bash "$SNAPSHOT_SCRIPT"
)

snapshot_file="$tmpdir/snapshot.json"

if [ ! -f "$snapshot_file" ]; then
    echo "Snapshot JSON was not created by $SNAPSHOT_SCRIPT" >&2
    exit 1
fi

if [ "$KEEP_SNAPSHOT_JSON" = "true" ]; then
    cp "$snapshot_file" "$SNAPSHOT_OUTPUT_PATH"
    echo "Saved snapshot copy to $SNAPSHOT_OUTPUT_PATH"
fi

python3 - "$snapshot_file" <<'PY'
import base64
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request


def env_bool(name: str, default: bool) -> bool:
    value = os.environ.get(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def request(method: str, url: str, headers: dict[str, str], payload: dict | None) -> dict:
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            body = response.read().decode("utf-8").strip()
            return json.loads(body) if body else {}
    except urllib.error.HTTPError as exc:
        details = exc.read().decode("utf-8", errors="replace").strip()
        if details:
            raise SystemExit(f"RabbitMQ API request failed: {exc.code} {exc.reason}: {details}")
        raise SystemExit(f"RabbitMQ API request failed: {exc.code} {exc.reason}")
    except urllib.error.URLError as exc:
        raise SystemExit(f"RabbitMQ API connection failed: {exc.reason}")


snapshot_path = sys.argv[1]
api_url = os.environ["RABBITMQ_API_URL"].rstrip("/")
user = os.environ["RABBITMQ_USER"]
password = os.environ["RABBITMQ_PASS"]
vhost = os.environ.get("RABBITMQ_VHOST", "/")
queue = os.environ.get("RABBITMQ_QUEUE", "sensor.snapshots")
routing_key = os.environ.get("RABBITMQ_ROUTING_KEY", queue)
durable = env_bool("RABBITMQ_DURABLE", True)
auto_delete = env_bool("RABBITMQ_AUTO_DELETE", False)
delivery_mode = int(os.environ.get("RABBITMQ_DELIVERY_MODE", "2"))

with open(snapshot_path, "r", encoding="utf-8") as handle:
    payload_text = handle.read()

json.loads(payload_text)

auth = base64.b64encode(f"{user}:{password}".encode("utf-8")).decode("ascii")
headers = {
    "Authorization": f"Basic {auth}",
    "Content-Type": "application/json",
    "Accept": "application/json",
}

vhost_enc = urllib.parse.quote(vhost, safe="")
queue_enc = urllib.parse.quote(queue, safe="")

request(
    "PUT",
    f"{api_url}/queues/{vhost_enc}/{queue_enc}",
    headers,
    {
        "auto_delete": auto_delete,
        "durable": durable,
        "arguments": {},
    },
)

result = request(
    "POST",
    f"{api_url}/exchanges/{vhost_enc}/amq.default/publish",
    headers,
    {
        "properties": {
            "content_type": "application/json",
            "delivery_mode": delivery_mode,
            "type": "sensor.snapshot",
        },
        "routing_key": routing_key,
        "payload": payload_text,
        "payload_encoding": "string",
    },
)

if not result.get("routed"):
    raise SystemExit(f"RabbitMQ accepted the publish request but did not route it to {routing_key!r}")

print(f"Published snapshot to queue {queue!r} via {api_url}")
PY
