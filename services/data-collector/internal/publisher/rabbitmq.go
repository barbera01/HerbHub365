package publisher

import (
	"context"

	"github.com/rabbitmq/amqp091-go"

	"herbhub365/services/data-collector/internal/config"
	"herbhub365/services/data-collector/internal/model"
)

type RabbitMQ struct {
	conn   *amqp091.Connection
	channe *amqp091.Channel
	config config.RabbitMQConfig
}

func NewRabbitMQ(cfg config.RabbitMQConfig) (*RabbitMQ, error) {
	conn, err := amqp091.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
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
		channel.Close()
		conn.Close()
		return nil, err
	}

	return &RabbitMQ{conn: conn, channe: channel, config: cfg}, nil
}

func (r *RabbitMQ) Publish(ctx context.Context, snapshot model.Snapshot) error {
	body, err := model.Marshal(snapshot)
	if err != nil {
		return err
	}

	mode := uint8(amqp091.Transient)
	if r.config.Persistent {
		mode = amqp091.Persistent
	}

	return r.channe.PublishWithContext(
		ctx,
		r.config.Exchange,
		r.config.RoutingKey,
		false,
		false,
		amqp091.Publishing{
			ContentType:  "application/json",
			Body:         body,
			MessageId:    snapshot.MessageID,
			Timestamp:    snapshot.Timestamp,
			DeliveryMode: mode,
			Type:         "sensor.snapshot",
		},
	)
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.channe != nil {
		_ = r.channe.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
