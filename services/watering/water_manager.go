package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	amqp "github.com/streadway/amqp"
)

// Constants
const (
	defaultMetricsURL  = "http://hh-02:9100/metrics"
	defaultRabbitMQURL = "amqp://admin:yourpassword@rabbitmq:5672/"
	Exchange           = "herbhub.watering"
	QueueName          = "watering.queue"
	MonitorInterval    = 5 * time.Minute
	ServerPort         = ":8787"
	defaultThreshold   = 40.0 // Default soil moisture threshold in percent
)

var (
	minSoilMoistureThreshold float64
	metricsURL               string
	rabbitMQURL              string
	promPattern              = regexp.MustCompile(`herbhub_soil_percent\{plant="([^"]+)"\}\s+([0-9.]+)`)
)

// PrometheusMetric holds parsed metric data
type PrometheusMetric struct {
	Plant string
	Value float64
}

func main() {
	// Load configuration from environment variables
	if envThreshold := os.Getenv("SOIL_MOISTURE_THRESHOLD"); envThreshold != "" {
		minSoilMoistureThreshold, _ = strconv.ParseFloat(envThreshold, 64)
	}
	if minSoilMoistureThreshold <= 0 {
		minSoilMoistureThreshold = defaultThreshold
	}

	metricsURL = os.Getenv("METRICS_URL"  - RABBITMQ_URL = amqp://admin:yourpassword@rabbitmq:5672/
  - METRICS_URL = http://hh-02:9100/metrics
  - SOIL_MOISTURE_THRESHOLD = 40.0)
	if metricsURL == "" {
		metricsURL = defaultMetricsURL
	}

	rabbitMQURL = os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = defaultRabbitMQURL
	}

	log.Printf("Initialized with moisture threshold: %.2f%%", minSoilMoistureThreshold)
	log.Printf("Metrics URL: %s", metricsURL)
	log.Printf("RabbitMQ URL: %s", rabbitMQURL)

	// Setup channels for signal handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start monitoring in background (manages its own connection with reconnect)
	go monitorLoop(ctx)

	// Start HTTP server for health checks
	go httpServer(ctx)

	// Wait for shutdown signal
	<-stop
	log.Println("Shutting down...")
	cancel()
	time.Sleep(2 * time.Second) // Give time for cleanup
}

func connectToRabbitMQ(ctx context.Context) (*amqp.Connection, error) {
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	log.Println("Connected to RabbitMQ")
	return conn, nil
}

func monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(MonitorInterval)
	defer ticker.Stop()

	log.Println("Starting monitoring loop")

	var conn *amqp.Connection

	connect := func() bool {
		if conn != nil && !conn.IsClosed() {
			return true
		}
		var err error
		conn, err = connectToRabbitMQ(ctx)
		if err != nil {
			log.Printf("RabbitMQ reconnect failed: %v", err)
			return false
		}
		return true
	}

	// Initial check
	if connect() {
		checkAndPost(ctx, conn)
	}

	for {
		select {
		case <-ticker.C:
			if connect() {
				checkAndPost(ctx, conn)
			}
		case <-ctx.Done():
			log.Println("Monitor loop stopped")
			if conn != nil {
				conn.Close()
			}
			return
		}
	}
}

func checkAndPost(ctx context.Context, rabbitConn *amqp.Connection) {
	log.Println("Fetching metrics...")

	// Fetch metrics from Prometheus
	metrics, err := fetchMetrics(ctx)
	if err != nil {
		log.Printf("Failed to fetch metrics: %v", err)
		return
	}

	// Create channel for RabbitMQ
	ch, err := rabbitConn.Channel()
	if err != nil {
		log.Printf("Failed to get RabbitMQ channel: %v", err)
		return
	}
	defer ch.Close()

	// Declare exchange (idempotent)
	err = ch.ExchangeDeclare(
		Exchange,
		"topic", // type
		true,    // durable
		false,   // auto-deleted
		false,   // internal
		false,   // no-wait
		nil,
	)
	if err != nil {
		log.Printf("Failed to declare exchange: %v", err)
		return
	}

	// Declare queue (idempotent)
	_, err = ch.QueueDeclare(
		QueueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Failed to declare queue: %v", err)
		return
	}

	// Bind queue to exchange with wildcard routing key
	err = ch.QueueBind(
		QueueName,
		"watering.#",
		Exchange,
		false,
		nil,
	)
	if err != nil {
		log.Printf("Failed to bind queue: %v", err)
		return
	}

	// Process each plant and make decisions
	for _, metric := range metrics {
		var decision struct {
			Plant  string  `json:"plant"`
			Action string  `json:"action"` // "water" or "skip"
			Value  float64 `json:"value"`  // current moisture value
		}

		if metric.Value < minSoilMoistureThreshold {
			decision = struct {
				Plant  string  `json:"plant"`
				Action string  `json:"action"`
				Value  float64 `json:"value"`
			}{
				Plant:  metric.Plant,
				Action: "water",
				Value:  metric.Value,
			}
			log.Printf("Plant %s needs water: %.2f%% (threshold: %.2f%%)", metric.Plant, metric.Value, minSoilMoistureThreshold)
		} else {
			decision = struct {
				Plant  string  `json:"plant"`
				Action string  `json:"action"`
				Value  float64 `json:"value"`
			}{
				Plant:  metric.Plant,
				Action: "skip",
				Value:  metric.Value,
			}
			log.Printf("Plant %s OK: %.2f%% (threshold: %.2f%%)", metric.Plant, metric.Value, minSoilMoistureThreshold)
		}

		// Send to RabbitMQ
		body, err := json.Marshal(decision)
		if err != nil {
			log.Printf("Failed to marshal decision: %v", err)
			continue
		}

		err = ch.Publish(
			Exchange, // exchange
			fmt.Sprintf("watering.%s", strings.ToLower(decision.Plant)), // routing key
			false, // mandatory
			false, // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        body,
			},
		)
		if err != nil {
			log.Printf("Failed to send message for %s: %v", decision.Plant, err)
		} else {
			log.Printf("Sent decision for %s to queue: %s", decision.Plant, decision.Action)
		}
	}
}

func fetchMetrics(ctx context.Context) ([]PrometheusMetric, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", metricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned status: %d", resp.StatusCode)
	}

	// Parse Prometheus text format
	var metrics []PrometheusMetric
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := promPattern.FindStringSubmatch(line)
		if len(matches) == 3 {
			plant := matches[1]
			value, err := strconv.ParseFloat(matches[2], 64)
			if err != nil {
				continue
			}

			metrics = append(metrics, PrometheusMetric{
				Plant: plant,
				Value: math.Round(value*100) / 100, // Round to 2 decimals
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read metrics: %w", err)
	}

	return metrics, nil
}

func httpServer(ctx context.Context) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})

	srv := &http.Server{
		Addr:         ServerPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Health server starting on %s", ServerPort)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown
	<-ctx.Done()
	log.Println("HTTP server shutting down...")

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctxWithTimeout)
}
