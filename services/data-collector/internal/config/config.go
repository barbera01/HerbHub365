package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Source           string
	DeviceName       string
	RunTimeout       time.Duration
	EmitJSONToStdout bool
	Sensor           SensorConfig
	RabbitMQ         RabbitMQConfig
}

type SensorConfig struct {
	I2CBus             int
	ADSAddress         uint16
	BME280Address      uint16
	BH1750Address      uint16
	TriggerPin         int
	EchoPin            int
	EmptyDistanceCM    float64
	FullDistanceCM     float64
	TempMap            map[string]string
	MoistureMap        map[string]string
	ADSChannelConfig   map[string]byte
	DryCalibration     map[string]float64
	WetCalibration     map[string]float64
	OneWireBasePath    string
	GPIOBasePath       string
	I2CDevicePath      string
	HCSR04Samples      int
	HCSR04Timeout      time.Duration
	BH1750Measurement  time.Duration
	ADSSettleDelay     time.Duration
	BME280MeasureDelay time.Duration
}

type RabbitMQConfig struct {
	URL        string
	QueueName  string
	Exchange   string
	RoutingKey string
	Persistent bool
}

func Load() Config {
	deviceName, _ := os.Hostname()

	queueName := getEnv("RABBITMQ_QUEUE", "sensor.snapshots")
	exchange := os.Getenv("RABBITMQ_EXCHANGE")
	routingKey := getEnv("RABBITMQ_ROUTING_KEY", queueName)

	return Config{
		Source:           getEnv("COLLECTOR_SOURCE", "data-collector"),
		DeviceName:       getEnv("DEVICE_NAME", deviceName),
		RunTimeout:       getDurationEnv("RUN_TIMEOUT", 10*time.Second),
		EmitJSONToStdout: getBoolEnv("EMIT_JSON_TO_STDOUT", false),
		Sensor: SensorConfig{
			I2CBus:          getIntEnv("I2C_BUS", 1),
			ADSAddress:      getHexUint16Env("ADS1115_ADDRESS", 0x48),
			BME280Address:   getHexUint16Env("BME280_ADDRESS", 0x76),
			BH1750Address:   getHexUint16Env("BH1750_ADDRESS", 0x23),
			TriggerPin:      getIntEnv("HCSR04_TRIGGER_PIN", 18),
			EchoPin:         getIntEnv("HCSR04_ECHO_PIN", 27),
			EmptyDistanceCM: getFloatEnv("WATER_EMPTY_DISTANCE_CM", 26.0),
			FullDistanceCM:  getFloatEnv("WATER_FULL_DISTANCE_CM", 4.0),
			TempMap: map[string]string{
				"28-0000006d0b68": "basil",
				"28-000000671c2b": "oregano",
				"28-00000071bd22": "chilli",
				"28-0000007131c5": "water",
			},
			MoistureMap: map[string]string{
				"A0": "oregano",
				"A1": "chilli",
				"A2": "basil",
			},
			ADSChannelConfig: map[string]byte{
				"A0": 0xC3,
				"A1": 0xD3,
				"A2": 0xE3,
				"A3": 0xF3,
			},
			DryCalibration: map[string]float64{
				"oregano": 2.170,
				"chilli":  2.137,
				"basil":   2.131,
			},
			WetCalibration: map[string]float64{
				"oregano": 1.014,
				"chilli":  0.828,
				"basil":   0.928,
			},
			OneWireBasePath:    getEnv("ONE_WIRE_BASE_PATH", "/sys/bus/w1/devices"),
			GPIOBasePath:       getEnv("GPIO_BASE_PATH", "/sys/class/gpio"),
			I2CDevicePath:      getEnv("I2C_DEVICE_TEMPLATE", "/dev/i2c-%d"),
			HCSR04Samples:      getIntEnv("HCSR04_SAMPLES", 5),
			HCSR04Timeout:      getDurationEnv("HCSR04_TIMEOUT", 1*time.Second),
			BH1750Measurement:  getDurationEnv("BH1750_MEASUREMENT_DELAY", 200*time.Millisecond),
			ADSSettleDelay:     getDurationEnv("ADS1115_SETTLE_DELAY", 10*time.Millisecond),
			BME280MeasureDelay: getDurationEnv("BME280_MEASURE_DELAY", 50*time.Millisecond),
		},
		RabbitMQ: RabbitMQConfig{
			URL:        getEnv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
			QueueName:  queueName,
			Exchange:   exchange,
			RoutingKey: routingKey,
			Persistent: getBoolEnv("RABBITMQ_PERSISTENT", true),
		},
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Sprintf("invalid integer for %s: %v", key, err))
	}
	return parsed
}

func getFloatEnv(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid float for %s: %v", key, err))
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		panic(fmt.Sprintf("invalid duration for %s: %v", key, err))
	}
	return parsed
}

func getBoolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		panic(fmt.Sprintf("invalid boolean for %s: %v", key, err))
	}
	return parsed
}

func getHexUint16Env(key string, fallback uint16) uint16 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	base := 10
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		base = 16
		value = value[2:]
	}
	parsed, err := strconv.ParseUint(value, base, 16)
	if err != nil {
		panic(fmt.Sprintf("invalid uint16 for %s: %v", key, err))
	}
	return uint16(parsed)
}
