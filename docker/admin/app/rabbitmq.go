package main

import (
	"log"

	"github.com/streadway/amqp"
)

// ConnectToRabbitMQ connects to RabbitMQ and returns a channel
func ConnectToRabbitMQ(amqpURL string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %s", err)
		return nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %s", err)
		return nil, nil, err
	}

	return conn, ch, nil
}
