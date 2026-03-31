#!/bin/bash

# RabbitMQ consumer script for herbhub watering
# Runs via cron to fetch messages from queue and execute relay wrapper
# Receives messages from: herbhub-watering service container

# Load environment file if it exists
if [ -f "/opt/herbhub/scripts/.env" ]; then
    source "/opt/herbhub/scripts/.env"
fi

# Configuration from environment variables with fallback defaults
RABBITMQ_HOST=${RABBITMQ_HOST:-hh-02}
RABBITMQ_PORT=${RABBITMQ_PORT:-5672}
RABBITMQ_USER=${RABBITMQ_USER:-admin}
RABBITMQ_PASS=${RABBITMQ_PASS:-yourpassword}
QUEUE_URL="amqp://${RABBITMQ_USER}:${RABBITMQ_PASS}@${RABBITMQ_HOST}:${RABBITMQ_PORT}/"
QUEUE_NAME="watering.queue"
LOG_FILE="${LOG_FILE:-/var/log/herbhub_watering.log}"
RELAY_SCRIPT_PATH="${RELAY_SCRIPT_PATH:-/opt/herbhub/scripts/relaywrapper.py}"

# Function to get single message from queue using python3 pika
get_queue_message() {
    local queue=${1:-$QUEUE_NAME}
    python3 -c "
import pika
import json
import sys

try:
    connection = pika.BlockingConnection(pika.URLParameters('$QUEUE_URL'))
    channel = connection.channel()
    channel.queue_declare(queue='$queue', durable=True)
    
    method_frame, header_frame, body = channel.basic_get(queue='$queue')
    if method_frame:
        print(body.decode())
        channel.basic_ack(method_frame.delivery_tag)
    
    connection.close()
except Exception as e:
    print(f'Error: {e}', file=sys.stderr)
    sys.exit(1)
" 2>/dev/null
}

# Process message
process_message() {
    local msg="$1"
    local plant=""
    local action=""
    local value=""
    
    # Extract plant and action from JSON message
    plant=$(echo "$msg" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('plant',''))" 2>/dev/null)
    action=$(echo "$msg" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('action',''))" 2>/dev/null)
    value=$(echo "$msg" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('value',''))" 2>/dev/null)
    
    if [ "$action" = "water" ] && [ -n "$plant" ]; then
        log_message "Watering $plant (moisture: $value%)"
        execute_relay "$plant"
    else
        log_message "Skipping: plant=$plant action=$action value=$value"
    fi
}

# Execute relay wrapper
execute_relay() {
    local plant="$1"
    
    log_message "Executing: sudo python3 $RELAY_SCRIPT_PATH $plant pulse 1"
    
    if sudo python3 "$RELAY_SCRIPT_PATH" "$plant" pulse 1 >> "$LOG_FILE" 2>&1; then
        log_message "Successfully watered $plant"
        return 0
    else
        log_message "Failed to water $plant"
        return 1
    fi
}

# Log messages with timestamp
log_message() {
    local msg="$1"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$timestamp] $msg" | tee -a "$LOG_FILE"
}

# Main function - single check (for cron usage)
main() {
    log_message "Consumer check started"
    
    # Get single message from queue
    msg=$(get_queue_message "$QUEUE_NAME")
    
    if [ -n "$msg" ]; then
        log_message "Received message: $msg"
        process_message "$msg"
    else
        log_message "No messages in queue"
    fi
    
    log_message "Consumer check completed"
}

# Run main function
main
