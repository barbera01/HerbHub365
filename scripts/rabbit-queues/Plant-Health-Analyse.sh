#!/bin/bash
# herbhub_rabbitmq_setup.sh — Create exchange, queue, and binding for plant analysis
# Usage: ./herbhub_rabbitmq_setup.sh

set -euo pipefail

# Load shared environment file if it exists
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
if [ -f "${SCRIPT_DIR}/.env" ]; then
    source "${SCRIPT_DIR}/.env"
fi

# --- Configuration ---
RABBITMQ_URL="${RABBITMQ_URL:-https://rabbit.herbhub365.com}"
RABBITMQ_USER="${RABBITMQ_USER:-admin}"
RABBITMQ_PASS="${RABBITMQ_PASS:-yourpassword}"
RABBITMQ_VHOST="${RABBITMQ_VHOST:-/}"

EXCHANGE="herbhub.plant"
QUEUE="plant.health.analysis"
DLX_EXCHANGE="herbhub.plant.dlx"
DLX_QUEUE="plant.health.analysis.dlq"

VHOST_ENCODED=$(python3 -c "import urllib.parse; print(urllib.parse.quote('${RABBITMQ_VHOST}', safe=''))")
API="${RABBITMQ_URL}/api"
AUTH="-u ${RABBITMQ_USER}:${RABBITMQ_PASS}"

call_api() {
  local method=$1 endpoint=$2 data=${3:-}
  local args=(-s -w "\n%{http_code}" -H "Content-Type: application/json" ${AUTH} -X "$method")
  [[ -n "$data" ]] && args+=(-d "$data")

  local response
  response=$(curl "${args[@]}" "${API}${endpoint}")
  local body=$(echo "$response" | sed '$d')
  local code=$(echo "$response" | tail -1)

  if [[ "$code" =~ ^2 ]]; then
    echo "  ✓ ${method} ${endpoint} (${code})"
  else
    echo "  ✗ ${method} ${endpoint} (${code})" >&2
    [[ -n "$body" ]] && echo "    ${body}" >&2
    return 1
  fi
}

echo "=== HerbHub RabbitMQ Setup ==="
echo "Target: ${RABBITMQ_URL} (vhost: ${RABBITMQ_VHOST})"
echo ""

# --- Dead Letter Exchange & Queue (catch failures) ---
echo "1. Dead letter exchange & queue"
call_api PUT "/exchanges/${VHOST_ENCODED}/${DLX_EXCHANGE}" \
  '{"type":"topic","durable":true,"auto_delete":false}'

call_api PUT "/queues/${VHOST_ENCODED}/${DLX_QUEUE}" \
  '{"durable":true,"auto_delete":false}'

call_api POST "/bindings/${VHOST_ENCODED}/e/${DLX_EXCHANGE}/q/${DLX_QUEUE}" \
  '{"routing_key":"#"}'

# --- Main Exchange ---
echo ""
echo "2. Main exchange"
call_api PUT "/exchanges/${VHOST_ENCODED}/${EXCHANGE}" \
  '{"type":"topic","durable":true,"auto_delete":false}'

# --- Main Queue (with DLX) ---
echo ""
echo "3. Analysis queue"
call_api PUT "/queues/${VHOST_ENCODED}/${QUEUE}" \
  "{\"durable\":true,\"auto_delete\":false,\"arguments\":{\"x-dead-letter-exchange\":\"${DLX_EXCHANGE}\",\"x-message-ttl\":86400000}}"

# --- Bindings ---
echo ""
echo "4. Bindings"

# Catch all plant health messages
call_api POST "/bindings/${VHOST_ENCODED}/e/${EXCHANGE}/q/${QUEUE}" \
  '{"routing_key":"plant.health.#"}'

# Per-zone bindings (if you want separate queues later)
for ZONE in left middle right; do
  call_api POST "/bindings/${VHOST_ENCODED}/e/${EXCHANGE}/q/${QUEUE}" \
    "{\"routing_key\":\"plant.health.${ZONE}\"}"
done

# --- Verify ---
echo ""
echo "5. Verification"
echo "   Exchange:"
curl -s ${AUTH} "${API}/exchanges/${VHOST_ENCODED}/${EXCHANGE}" | jq '{name, type, durable}'
echo "   Queue:"
curl -s ${AUTH} "${API}/queues/${VHOST_ENCODED}/${QUEUE}" | jq '{name, durable, messages, consumers}'
echo "   Bindings:"
curl -s ${AUTH} "${API}/queues/${VHOST_ENCODED}/${QUEUE}/bindings" | jq '.[].routing_key'

echo ""
echo "=== Setup complete ==="
echo ""
echo "Test with:"
echo "  curl -s -u \${RABBITMQ_USER}:\${RABBITMQ_PASS} \\"
echo "    -H 'Content-Type: application/json' -X POST \\"
echo "    -d '{\"properties\":{},\"routing_key\":\"plant.health.left\",\"payload\":\"{\\\"test\\\":true}\",\"payload_encoding\":\"string\"}' \\"
echo "    ${RABBITMQ_URL}/api/exchanges/${VHOST_ENCODED}/${EXCHANGE}/publish"
