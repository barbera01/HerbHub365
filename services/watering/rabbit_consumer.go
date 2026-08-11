package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	defaultRabbitQueue = "watering.queue"
	defaultDedupeFile  = "/var/lib/herbhub-watering/completed.json"
	maxWaterMessageAge = 5 * time.Minute
	maxFutureSkew      = 1 * time.Minute
	minReconnectDelay  = 1 * time.Second
	maxReconnectDelay  = 30 * time.Second
)

var errFatalWateringSafety = errors.New("fatal watering safety error")

type rabbitConfig struct {
	url   string
	queue string
}

type wateringMessage struct {
	Plant  string  `json:"plant"`
	Action string  `json:"action"`
	Value  float64 `json:"value"`
}

type wateringMessageRaw struct {
	Plant  string   `json:"plant"`
	Action string   `json:"action"`
	Value  *float64 `json:"value"`
}

type pulseController interface {
	PulseSync(ctx context.Context, channels []string, d time.Duration) error
}

type acknowledger interface {
	Ack(multiple bool) error
	Reject(requeue bool) error
}

type completionLedger interface {
	IsComplete(messageID string) bool
	MarkComplete(messageID string) error
}

func runRabbitConsumer(ctx context.Context, ctrl pulseController, pulseDuration time.Duration) error {
	cfg := rabbitConfig{
		url:   strings.TrimSpace(os.Getenv("RABBITMQ_URL")),
		queue: strings.TrimSpace(getEnv("RABBITMQ_QUEUE", defaultRabbitQueue)),
	}

	if cfg.url == "" {
		log.Printf("rabbit consumer disabled: RABBITMQ_URL is not set")
		return nil
	}
	if cfg.queue == "" {
		cfg.queue = defaultRabbitQueue
	}

	dedupeFile := strings.TrimSpace(getEnv("WATERING_DEDUPE_FILE", defaultDedupeFile))
	ledger, err := newFileCompletionLedger(dedupeFile)
	if err != nil {
		return fmt.Errorf("init watering dedupe ledger %q: %w", dedupeFile, err)
	}

	log.Printf("rabbit consumer enabled: queue=%s dedupe_file=%s", cfg.queue, dedupeFile)
	return runRabbitConsumerLoop(ctx, cfg, ctrl, pulseDuration, ledger, consumeOnce)
}

type consumeOnceFunc func(ctx context.Context, cfg rabbitConfig, ctrl pulseController, pulseDuration time.Duration, ledger completionLedger) error

func runRabbitConsumerLoop(ctx context.Context, cfg rabbitConfig, ctrl pulseController, pulseDuration time.Duration, ledger completionLedger, consume consumeOnceFunc) error {
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := consume(ctx, cfg, ctrl, pulseDuration, ledger)
		if err == nil || errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, errFatalWateringSafety) {
			log.Printf("rabbit consumer fatal safety stop: %v", err)
			return err
		}

		attempt++
		delay := backoffDelay(attempt, minReconnectDelay, maxReconnectDelay)
		log.Printf("rabbit consumer disconnected: %v; reconnecting in %s", err, delay)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func consumeOnce(ctx context.Context, cfg rabbitConfig, ctrl pulseController, pulseDuration time.Duration, ledger completionLedger) error {
	conn, err := amqp.Dial(cfg.url)
	if err != nil {
		return fmt.Errorf("amqp dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("amqp channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("amqp qos: %w", err)
	}

	if _, err := ch.QueueDeclarePassive(cfg.queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("amqp passive queue %q: %w", cfg.queue, err)
	}

	consumerTag := fmt.Sprintf("watering-%d", time.Now().UnixNano())
	msgs, err := ch.Consume(cfg.queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("amqp consume: %w", err)
	}

	chClosed := make(chan *amqp.Error, 1)
	connClosed := make(chan *amqp.Error, 1)
	ch.NotifyClose(chClosed)
	conn.NotifyClose(connClosed)

	for {
		select {
		case <-ctx.Done():
			_ = ch.Cancel(consumerTag, false)
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return errors.New("amqp deliveries channel closed")
			}
			if err := handleWateringDelivery(ctx, ctrl, pulseDuration, msg, ledger); err != nil {
				return err
			}
		case e := <-chClosed:
			if e == nil {
				return errors.New("amqp channel closed")
			}
			return fmt.Errorf("amqp channel closed: %w", e)
		case e := <-connClosed:
			if e == nil {
				return errors.New("amqp connection closed")
			}
			return fmt.Errorf("amqp connection closed: %w", e)
		}
	}
}

func handleWateringDelivery(ctx context.Context, ctrl pulseController, pulseDuration time.Duration, msg amqp.Delivery, ledger completionLedger) error {
	return handleWateringPayload(ctx, ctrl, pulseDuration, msg.Body, msg.MessageId, msg.Timestamp, msg, ledger, time.Now)
}

func handleWateringPayload(ctx context.Context, ctrl pulseController, pulseDuration time.Duration, body []byte, messageID string, timestamp time.Time, ack acknowledger, ledger completionLedger, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}

	payload, err := parseWateringMessage(body)
	if err != nil {
		log.Printf("rejecting invalid watering message: %v", err)
		if rejErr := ack.Reject(false); rejErr != nil {
			return fmt.Errorf("reject invalid message: %w", rejErr)
		}
		return nil
	}

	switch payload.Action {
	case "skip":
		log.Printf("watering queue: skip plant=%s moisture=%.2f", payload.Plant, payload.Value)
		if err := ack.Ack(false); err != nil {
			return fmt.Errorf("ack skip message: %w", err)
		}
		return nil
	case "water":
		messageID = strings.TrimSpace(messageID)
		if reason, ok := validateWaterDeliveryMetadata(messageID, timestamp, now()); !ok {
			log.Printf("watering queue: rejecting water delivery plant=%s reason=%s", payload.Plant, reason)
			if rejErr := ack.Reject(false); rejErr != nil {
				return fmt.Errorf("reject water delivery metadata invalid: %w", rejErr)
			}
			return nil
		}

		if ledger != nil && ledger.IsComplete(messageID) {
			log.Printf("watering queue: duplicate suppressed plant=%s message_id=%s", payload.Plant, messageID)
			if err := ack.Ack(false); err != nil {
				return fmt.Errorf("ack duplicate water message: %w", err)
			}
			return nil
		}

		log.Printf("watering queue: water plant=%s moisture=%.2f duration=%s", payload.Plant, payload.Value, pulseDuration)
		if err := ctrl.PulseSync(ctx, []string{payload.Plant}, pulseDuration); err != nil {
			log.Printf("watering queue: pulse failed plant=%s: %v", payload.Plant, err)
			if rejErr := ack.Reject(false); rejErr != nil {
				return fmt.Errorf("reject message after pulse failure: %w", rejErr)
			}
			return nil
		}
		log.Printf("watering queue: pulse completed plant=%s", payload.Plant)
		// Unavoidable narrow crash window: if the process crashes after relay-off
		// but before the durable ledger write commits, a redelivery may trigger one
		// additional pulse because completion evidence was not recorded yet.
		if messageID != "" && ledger != nil {
			if err := ledger.MarkComplete(messageID); err != nil {
				log.Printf("watering queue: dedupe persist failed plant=%s message_id=%s: %v", payload.Plant, messageID, err)
				if rejErr := ack.Reject(false); rejErr != nil {
					return fmt.Errorf("%w: reject message after dedupe persist failure: %v", errFatalWateringSafety, rejErr)
				}
				return fmt.Errorf("%w: dedupe persist failed for water message_id=%s: %v", errFatalWateringSafety, messageID, err)
			}
		}
		if err := ack.Ack(false); err != nil {
			return fmt.Errorf("ack water message: %w", err)
		}
		return nil
	default:
		// Should never happen due to parseWateringMessage validation.
		if rejErr := ack.Reject(false); rejErr != nil {
			return fmt.Errorf("reject unsupported action: %w", rejErr)
		}
		return nil
	}
}

func validateWaterDeliveryMetadata(messageID string, timestamp time.Time, now time.Time) (string, bool) {
	if messageID == "" {
		return "missing_message_id", false
	}
	if timestamp.IsZero() {
		return "missing_timestamp", false
	}
	age := now.Sub(timestamp)
	if age < -maxFutureSkew {
		return "timestamp_too_far_in_future", false
	}
	if age > maxWaterMessageAge {
		return "timestamp_too_old", false
	}
	return "", true
}

func parseWateringMessage(body []byte) (wateringMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	var raw wateringMessageRaw
	if err := dec.Decode(&raw); err != nil {
		return wateringMessage{}, fmt.Errorf("invalid json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return wateringMessage{}, errors.New("invalid json: trailing data")
	}

	if strings.TrimSpace(raw.Plant) == "" {
		return wateringMessage{}, errors.New("plant is required")
	}
	if strings.TrimSpace(raw.Action) == "" {
		return wateringMessage{}, errors.New("action is required")
	}
	if raw.Value == nil {
		return wateringMessage{}, errors.New("value is required")
	}

	msg := wateringMessage{
		Plant:  strings.ToUpper(strings.TrimSpace(raw.Plant)),
		Action: strings.ToLower(strings.TrimSpace(raw.Action)),
		Value:  *raw.Value,
	}

	if _, ok := relays[msg.Plant]; !ok {
		return wateringMessage{}, fmt.Errorf("unknown plant %q", msg.Plant)
	}
	if msg.Action != "skip" && msg.Action != "water" {
		return wateringMessage{}, fmt.Errorf("unknown action %q", msg.Action)
	}
	if math.IsNaN(msg.Value) || math.IsInf(msg.Value, 0) {
		return wateringMessage{}, errors.New("value must be finite")
	}
	if msg.Value < 0 || msg.Value > 100 {
		return wateringMessage{}, errors.New("value out of range [0,100]")
	}

	return msg, nil
}

func backoffDelay(attempt int, minDelay, maxDelay time.Duration) time.Duration {
	if attempt <= 0 {
		return minDelay
	}
	d := minDelay
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= maxDelay {
			return maxDelay
		}
	}
	if d > maxDelay {
		return maxDelay
	}
	return d
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
