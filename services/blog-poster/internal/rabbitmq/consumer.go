package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"HerbHub365/services/blog-poster/internal/archive"
	"HerbHub365/services/blog-poster/internal/config"
	"HerbHub365/services/blog-poster/internal/model"
	"github.com/rabbitmq/amqp091-go"
)

var ErrNoMessages = errors.New("no messages available")

type Consumer struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel
	cfg     config.RabbitMQConfig
}

func NewConsumer(cfg config.RabbitMQConfig) (*Consumer, error) {
	conn, err := amqp091.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := channel.Qos(cfg.Prefetch, 0, false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	if _, err := channel.QueueDeclare(
		cfg.QueueName,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	return &Consumer{conn: conn, channel: channel, cfg: cfg}, nil
}

func (c *Consumer) Run(ctx context.Context, store *archive.Store) error {
	deliveries, err := c.channel.Consume(
		c.cfg.QueueName,
		c.cfg.ConsumerTag,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}

			var snapshot model.Snapshot
			if err := json.Unmarshal(delivery.Body, &snapshot); err != nil {
				_ = delivery.Ack(false)
				continue
			}

			if err := store.Append(snapshot, delivery.Body); err != nil {
				_ = delivery.Nack(false, true)
				return err
			}

			if err := delivery.Ack(false); err != nil {
				return err
			}
		}
	}
}

func (c *Consumer) FetchSnapshot(requeue bool) (*model.Snapshot, []byte, error) {
	delivery, ok, err := c.channel.Get(c.cfg.QueueName, false)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrNoMessages
	}

	var snapshot model.Snapshot
	if err := json.Unmarshal(delivery.Body, &snapshot); err != nil {
		if requeue {
			_ = delivery.Nack(false, true)
		} else {
			_ = delivery.Reject(false)
		}
		return nil, nil, err
	}

	if requeue {
		if err := delivery.Nack(false, true); err != nil {
			return nil, nil, err
		}
	} else {
		if err := delivery.Ack(false); err != nil {
			return nil, nil, err
		}
	}

	return &snapshot, delivery.Body, nil
}

func (c *Consumer) Close() error {
	if c == nil {
		return nil
	}
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
