#!/bin/bash

# Setup cron job for watering consumer
# This script should be run on the remote machine (hh-02) where the relays are connected

CONSUMER_SCRIPT="/opt/herbhub/scripts/consumer.sh"
CRON_SCHEDULE="*/5 * * * *"  # Every 5 minutes

echo "Setting up watering consumer cron job..."

# Make consumer script executable
chmod +x "$CONSUMER_SCRIPT"

# Check for environment file
if [ ! -f "/opt/herbhub/scripts/.env" ]; then
    echo "WARNING: No environment file found at /opt/herbhub/scripts/.env"
    echo "Consider copying scripts/consumer.env.example to /opt/herbhub/scripts/.env and customizing values"
fi

# Add cron job if it doesn't exist
CRON_LINE="$CRON_SCHEDULE $CONSUMER_SCRIPT"
(crontab -l 2>/dev/null | grep -F "$CONSUMER_SCRIPT") || (
    echo "Adding cron job: $CRON_LINE"
    (crontab -l 2>/dev/null; echo "$CRON_LINE") | crontab -
)

echo "Watering consumer cron job setup complete"
echo "Schedule: $CRON_SCHEDULE"
echo "Script: $CONSUMER_SCRIPT"

# Test the consumer script
echo "Testing consumer script..."
if [ -x "$CONSUMER_SCRIPT" ]; then
    echo "Consumer script is executable"
    echo "You can test it manually with: $CONSUMER_SCRIPT"
else
    echo "WARNING: Consumer script is not executable at $CONSUMER_SCRIPT"
fi

echo ""
echo "Environment Configuration:"
echo "- Copy scripts/consumer.env.example to /opt/herbhub/scripts/.env"
echo "- Customize RabbitMQ credentials and paths in the .env file"
echo "- Consumer will use environment variables or fall back to defaults"