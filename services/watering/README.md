# HerbHub365 Watering System

An automated watering system that monitors soil moisture via Prometheus metrics and controls relays via RabbitMQ messaging.

## Architecture

### Components

1. **Water Manager Service** (`services/watering/`)
   - Go service that runs in Docker
   - Fetches soil moisture metrics from `http://hh-02:9100/metrics`
   - Makes watering decisions based on configurable threshold
   - Posts decisions to RabbitMQ queue

2. **Consumer Script** (`scripts/consumer.sh`)
   - Bash script that runs via cron on remote machine (hh-02)
   - Consumes messages from RabbitMQ queue
   - Executes relay wrapper to control watering pumps

3. **Relay Wrapper** (`scripts/relaywrapper.py`)
   - Python script that controls GPIO relays
   - Controls individual plant watering pumps
   - Supports pulse commands for timed watering

## Configuration

### Environment Variables

- `WATERING_SOIL_MOISTURE_THRESHOLD`: Soil moisture threshold percentage (default: 40.0)

### RabbitMQ

- **URL**: `amqp://admin:yourpassword@hh-02:5672/`
- **Exchange**: `herbhub.watering` (topic)
- **Queue**: `watering.queue`
- **Dead Letter Queue**: `watering.queue.dlq`
- **Routing Keys**: 
  - `watering.basil` - Basil plant watering
  - `watering.chilli` - Chilli plant watering  
  - `watering.oregano` - Oregano plant watering
  - `watering.#` - All watering messages
- **Message format**:
  ```json
  {
    "plant": "basil",
    "action": "water",
    "value": 35.2
  }
  ```

## Deployment

### Docker Service

The watering service is defined in `docker/docker-compose.yml` and will start automatically with the stack.

### Consumer Script Setup

On the remote machine (hh-02) where relays are connected:

1. Copy `scripts/consumer.sh` to `/opt/herbhub/scripts/consumer.sh`
2. Copy `scripts/relaywrapper.py` to `/opt/herbhub/scripts/relaywrapper.py`
3. Install Python dependencies: `pip3 install pika gpiod`
4. Run setup script: `./scripts/setup-watering-cron.sh`

This will create a cron job that runs every 5 minutes to check for watering messages.

## Plant Mappings

The system recognizes these plants (from `relaywrapper.py`):

- **BASIL** → GPIO 26
- **CHILLI** → GPIO 20  
- **OREGANO** → GPIO 21

## Monitoring

### Health Checks

- **Water Manager**: `http://localhost:8787/health`
- **Logs**: `docker logs watering`

### Consumer Logs

- Location: `/var/log/herbhub_watering.log`
- Contains execution logs and error messages

## Flow

1. Water Manager fetches metrics every 5 minutes from Prometheus
2. For each plant, if soil moisture < threshold:
   - Creates "water" message
   - Posts to RabbitMQ queue
3. Consumer script (cron every 5 minutes):
   - Checks queue for messages
   - If "water" action found, executes relay wrapper
   - Relay wrapper pulses GPIO pin for 1 second

## Manual Testing

### Test Water Manager

```bash
# Check metrics parsing
curl http://hh-02:9100/metrics | grep herbhub_soil_percent

# Check health
curl http://localhost:8787/health
```

### Test Consumer

```bash
# Manual consumer run
sudo /opt/herbhub/scripts/consumer.sh

# Manual relay test  
sudo python3 /opt/herbhub/scripts/relaywrapper.py basil pulse 1
```

### Test Queue

```bash
# List queue status (new location)
./scripts/rabbit-queues/watering.sh -v

# Test message publishing
curl -s -u admin:yourpassword \
  -H 'Content-Type: application/json' -X POST \
  -d '{"properties":{},"routing_key":"watering.basil","payload":"{\"plant\":\"basil\",\"action\":\"water\",\"value\":35.2}","payload_encoding":"string"}' \
  https://rabbit.herbhub365.com/api/exchanges/%2F/herbhub.watering/publish
```