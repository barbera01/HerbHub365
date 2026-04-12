package queue

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher sends JSON messages to RabbitMQ.
type Publisher struct {
	url   string
	queue string
	conn  *amqp.Connection
	ch    *amqp.Channel
}

func NewPublisher(url, queue string) *Publisher {
	return &Publisher{url: url, queue: queue}
}

func (p *Publisher) Enabled() bool {
	return p != nil && p.url != "" && p.queue != ""
}

func (p *Publisher) ensureChannel() error {
	if !p.Enabled() {
		return fmt.Errorf("rabbitmq not configured")
	}

	if p.conn != nil && !p.conn.IsClosed() && p.ch != nil {
		return nil
	}

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("amqp channel: %w", err)
	}
	if _, err := ch.QueueDeclare(
		p.queue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("declare queue: %w", err)
	}

	p.conn = conn
	p.ch = ch
	return nil
}

func (p *Publisher) Publish(ctx context.Context, body []byte) error {
	if err := p.ensureChannel(); err != nil {
		return err
	}

	return p.ch.PublishWithContext(
		ctx,
		"",
		p.queue,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now().UTC(),
			DeliveryMode: amqp.Persistent,
		},
	)
}
