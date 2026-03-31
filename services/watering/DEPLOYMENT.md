# HerbHub365 Watering System - Deployment Checklist

## ✅ Completed

### 1. **Water Manager Service (Docker Container)**
- **Location**: `services/watering/`
- **Built**: Go service successfully compiles
- **Configuration**: Uses environment variables (aligned with other services)
- **Docker**: Added to `docker-compose.yml` with proper networking and health checks
- **Features**:
  - Fetches soil moisture from Prometheus (`http://hh-02:9100/metrics`)
  - Makes watering decisions based on configurable threshold
  - Posts decisions to RabbitMQ exchange (`herbhub.watering`) with plant-specific routing keys
  - Health endpoint: `/health` and `/ready`

### 2. **Consumer Script (Remote Machine)**
- **Location**: `scripts/consumer.sh`
- **Purpose**: Runs via cron on hh-02 to consume RabbitMQ messages
- **Features**:
  - Connects to RabbitMQ queue (`watering.queue`)
  - Processes "water" action messages
  - Executes relay wrapper for actual watering

### 3. **Queue Setup Script**
- **Location**: `scripts/rabbit-queues/watering.sh`
- **Aligned**: Matches pattern used by `Plant-Health-Analyse.sh`
- **Features**:
  - Creates exchange (`herbhub.watering`)
  - Creates queue with dead letter queue (`watering.queue` → `watering.queue.dlq`)
  - Sets up routing keys for each plant (`watering.basil`, `watering.chilli`, etc.)

### 4. **Setup Scripts**
- **Cron Setup**: `scripts/setup-watering-cron.sh`
- **Both**: Made executable and ready for deployment

### 5. **Documentation**
- **README**: Complete setup and deployment instructions
- **Architecture**: Clear separation between decision service and execution

## 🔧 Deployment Steps Required

### Step 1: Deploy Water Manager Service
```bash
cd /Users/andybarber/repos/HerbHub365/docker
docker-compose up watering
```

### Step 2: Setup RabbitMQ Exchange and Queue
```bash
./scripts/rabbit-queues/watering.sh
```

### Step 3: Deploy Consumer on hh-02
```bash
# Copy scripts to hh-02
scp scripts/consumer.sh hh-02:/opt/herbhub/scripts/
scp scripts/consumer.env.example hh-02:/opt/herbhub/scripts/
scp scripts/relaywrapper.py hh-02:/opt/herbhub/scripts/  
scp scripts/setup-watering-cron.sh hh-02:/tmp/

# On hh-02: Setup environment and cron
ssh hh-02 '
  # Install dependencies
  pip3 install pika
  
  # Setup environment file
  cp /opt/herbhub/scripts/consumer.env.example /opt/herbhub/scripts/.env
  # Edit .env file with actual credentials:
  # nano /opt/herbhub/scripts/.env
  
  # Setup cron job
  sudo /tmp/setup-watering-cron.sh
'
```

### Step 4: Test End-to-End
```bash
# Verify water manager is running
curl http://localhost:8787/health

# Check RabbitMQ queue
./scripts/rabbit-queues/watering.sh -v

# Check consumer logs on hh-02
ssh hh-02 'tail -f /var/log/herbhub_watering.log'
```

## 🌿 System Architecture

```
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│   Prometheus        │    │  Water Manager      │    │   RabbitMQ          │
│   (hh-02:9100)     │───▶│   (Docker)          │───▶│   Exchange:         │
│                     │    │                     │    │   herbhub.watering  │
│   soil_percent      │    │ - Fetch metrics     │    │                     │
│   basil: 35.2%      │    │ - Make decisions    │    │   Queue:            │
│   chilli: 28.1%     │    │ - Post to exchange  │    │   watering.queue    │
│   oregano: 45.6%    │    │ - Use routing keys  │    │                     │
└─────────────────────┘    └─────────────────────┘    └─────────────────────┘
                                                                │
                           ┌─────────────────────────────────────┘
                           │ Routing Keys:
                           │ • watering.basil
                           │ • watering.chilli  
                           │ • watering.oregano
                           ▼
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│   GPIO Relays       │    │   Relay Wrapper     │    │   Consumer Script   │
│   (hh-02)          │◀───│   relaywrapper.py   │◀───│   consumer.sh       │
│                     │    │                     │    │   (cron every 5min) │
│   BASIL → GPIO 26   │    │ - Control pumps     │    │ - Read queue        │
│   CHILLI → GPIO 20  │    │ - Pulse 1 second    │    │ - Execute watering  │
│   OREGANO → GPIO 21 │    │                     │    │                     │
└─────────────────────┘    └─────────────────────┘    └─────────────────────┘
```

## 🎯 Expected Behavior

1. **Every 5 minutes**: Water Manager fetches soil moisture from Prometheus
2. **Decision Logic**: If moisture < 40% (configurable), post message to `herbhub.watering` exchange with routing key `watering.{plant}`
3. **Every 5 minutes**: Consumer script checks queue for messages
4. **Execution**: For "water" messages, consumer executes: `python3 relaywrapper.py PLANT pulse 1`
5. **Result**: GPIO relay activates pump for 1 second to water the plant

## 🛠 Configuration

### Environment Variables (docker-compose)
```yaml
WATERING_SOIL_MOISTURE_THRESHOLD: 40.0    # Moisture threshold %
WATERING_RABBITMQ_URL: amqp://admin:yourpassword@rabbitmq:5672/
WATERING_METRICS_URL: http://hh-02:9100/metrics
```

### RabbitMQ Structure (aligned with plant analysis)
- **Exchange**: `herbhub.watering` (topic, durable)
- **Queue**: `watering.queue` (durable, with DLX)
- **Dead Letter Queue**: `watering.queue.dlq`
- **Routing Keys**: `watering.basil`, `watering.chilli`, `watering.oregano`, `watering.#`

### Cron Schedule
```bash
*/5 * * * * /opt/herbhub/scripts/consumer.sh  # Every 5 minutes
```

The system is now **fully aligned** with existing HerbHub365 queue patterns and ready for deployment!