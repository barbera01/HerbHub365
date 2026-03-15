# Data Collector

`services/data-collector` replaces the old `scripts/sensorsnapshot.sh` flow with a Go service that reads local sensors, builds a snapshot payload, and publishes that payload to RabbitMQ.

## What it does

- reads environmental data from `BME280` and `BH1750`
- reads water reservoir distance from `HC-SR04`
- reads one-wire temperatures from `DS18B20` devices under `/sys/bus/w1/devices`
- reads soil moisture voltages from `ADS1115` and converts them to calibrated percentages
- assembles a single JSON snapshot message
- publishes the snapshot to a RabbitMQ queue
- prints a terminal-friendly status summary for local runs

The app is organized by capability so each sensor can evolve independently:

- `cmd/data-collector`: application entrypoint
- `internal/config`: environment-driven configuration and defaults
- `internal/collector`: orchestration for one collection cycle
- `internal/model`: snapshot message schema
- `internal/publisher`: RabbitMQ publishing
- `internal/sensors`: one module per sensor or low-level hardware capability

## Message shape

The published message keeps the same core structure as the old `snapshot.json`, with a few metadata fields added for queue consumers.

```json
{
  "timestamp": "2026-03-13T12:00:00Z",
  "source": "data-collector",
  "device": "raspberrypi",
  "schema_version": "1.0",
  "message_id": "raspberrypi-1741867200000000000",
  "collected_at": "2026-03-13T12:00:00Z",
  "environment": {
    "temperature": 21.63,
    "humidity": 58.4,
    "pressure": 1013,
    "light_lux": 247.5
  },
  "water_reservoir": {
    "distance_cm": 9.42,
    "percent_full": 75.4,
    "volume_ml": 754
  },
  "temperatures": {
    "basil": 20.625,
    "oregano": 20.125,
    "chilli": 21.000,
    "water": 19.875
  },
  "soil_moisture": {
    "basil": {
      "voltage": 1.041,
      "percent": 90.4
    },
    "chilli": {
      "voltage": 1.332,
      "percent": 61.7
    },
    "oregano": {
      "voltage": 1.118,
      "percent": 90.9
    }
  },
  "warnings": []
}
```

If a sensor cannot be read, that field is left `null` in JSON and the failure is appended to `warnings`.

## Run locally

From `services/data-collector`:

```bash
go run ./cmd/data-collector
```

Build the binary:

```bash
go build ./cmd/data-collector
```

Build the container image:

```bash
docker build -t herbhub365/data-collector -f dockerfile .
```

The service expects direct access to:

- `/dev/i2c-*` for I2C sensors
- `/sys/bus/w1/devices` for one-wire temperature probes
- `/sys/class/gpio` for the ultrasonic sensor GPIO pins

It is intended to run on the Raspberry Pi that is physically attached to the sensors.

## Docker and Compose

The service now includes:

- `services/data-collector/dockerfile` for a multi-stage Go build
- `services/docker-compose.yml` entry for `data-collector`
- `services/data-collector/.env.example` as a starting point for deployment values

Suggested setup:

```bash
cp services/data-collector/.env.example services/data-collector/.env
```

Then run Compose from the `services` directory so `./data-collector/.env` resolves correctly.

The compose service mounts the required hardware paths from the host:

- `/dev/i2c-1`
- `/sys/bus/w1/devices`
- `/sys/class/gpio`

Because the ultrasonic sensor currently uses sysfs GPIO export/unexport, the container is marked `privileged: true`.
The container also runs as `root` so it can write to GPIO sysfs paths.

## Configuration

Configuration is environment-driven. Defaults mirror the original shell script where possible.

### RabbitMQ

| Variable | Default | Purpose |
| --- | --- | --- |
| `RABBITMQ_URL` | `amqp://guest:guest@rabbitmq:5672/` | RabbitMQ connection string |
| `RABBITMQ_QUEUE` | `sensor.snapshots` | Queue to declare and publish to |
| `RABBITMQ_EXCHANGE` | empty | Exchange name, if not using the default exchange |
| `RABBITMQ_ROUTING_KEY` | same as queue | Routing key used on publish |
| `RABBITMQ_PERSISTENT` | `true` | Publishes messages as persistent when enabled |

### General app settings

| Variable | Default | Purpose |
| --- | --- | --- |
| `COLLECTOR_SOURCE` | `data-collector` | Source name included in the message |
| `DEVICE_NAME` | system hostname | Device identifier included in the message |
| `RUN_TIMEOUT` | `10s` | Overall timeout for one collection and publish cycle |
| `EMIT_JSON_TO_STDOUT` | `false` | Also print the JSON payload after publish |

### Sensor settings

| Variable | Default |
| --- | --- |
| `I2C_BUS` | `1` |
| `ADS1115_ADDRESS` | `0x48` |
| `BME280_ADDRESS` | `0x76` |
| `BH1750_ADDRESS` | `0x23` |
| `HCSR04_TRIGGER_PIN` | `18` |
| `HCSR04_ECHO_PIN` | `27` |
| `WATER_EMPTY_DISTANCE_CM` | `26.0` |
| `WATER_FULL_DISTANCE_CM` | `4.0` |
| `ONE_WIRE_BASE_PATH` | `/sys/bus/w1/devices` |
| `GPIO_BASE_PATH` | `/sys/class/gpio` |
| `I2C_DEVICE_TEMPLATE` | `/dev/i2c-%d` |
| `HCSR04_SAMPLES` | `5` |
| `HCSR04_TIMEOUT` | `1s` |
| `BH1750_MEASUREMENT_DELAY` | `200ms` |
| `ADS1115_SETTLE_DELAY` | `10ms` |
| `BME280_MEASURE_DELAY` | `50ms` |

Sensor-to-plant mappings and moisture calibration constants currently live in `internal/config/config.go`.

## Notes on hardware support

- `DS18B20` uses the Linux one-wire sysfs interface instead of a third-party driver.
- `ADS1115`, `BH1750`, and `BME280` are read over Linux I2C directly.
- `HC-SR04` currently uses the sysfs GPIO interface. If the target Pi image moves fully to `libgpiod`, this module should be updated.
- The collector tolerates partial failures and still publishes whatever data it was able to collect.

## Development

Format and build from `services/data-collector`:

```bash
gofmt -w .
go build ./...
```

Suggested next improvements:

- move plant mappings and calibration tables into config files or env-driven JSON
- add a dry-run mode with mock sensor readers for local development
- add integration docs for the RabbitMQ consumer side
