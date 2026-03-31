# RabbitMQ Queue Setup Scripts

This directory contains scripts for setting up RabbitMQ exchanges, queues, and bindings for HerbHub365 services.

## Scripts

- **`Plant-Health-Analyse.sh`** - Sets up plant health analysis queue infrastructure
- **`watering.sh`** - Sets up watering system queue infrastructure

## Environment Configuration

### Shared Configuration

All scripts can use a shared environment file:

1. Copy the example: `cp .env.example .env`
2. Edit with your credentials: `nano .env`

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RABBITMQ_URL` | `https://rabbit.herbhub365.com` | RabbitMQ management API URL |
| `RABBITMQ_USER` | `admin` | RabbitMQ username |
| `RABBITMQ_PASS` | `yourpassword` | RabbitMQ password |
| `RABBITMQ_VHOST` | `/` | RabbitMQ virtual host |

### Usage

```bash
# Setup environment (one time)
cp .env.example .env
nano .env  # Edit credentials

# Run setup scripts
./Plant-Health-Analyse.sh
./watering.sh
```

## Queue Structure

### Plant Health Analysis
- **Exchange**: `herbhub.plant` (topic)
- **Queue**: `plant.health.analysis`
- **Dead Letter**: `plant.health.analysis.dlq`
- **Routing Keys**: `plant.health.#`, `plant.health.left`, `plant.health.middle`, `plant.health.right`

### Watering System  
- **Exchange**: `herbhub.watering` (topic)
- **Queue**: `watering.queue`
- **Dead Letter**: `watering.queue.dlq`
- **Routing Keys**: `watering.#`, `watering.basil`, `watering.chilli`, `watering.oregano`

## Security

- ✅ All scripts use environment variables with fallback defaults
- ✅ Shared `.env` file for credential management
- ✅ Default credentials are placeholder values
- ✅ Production deployments should always override defaults