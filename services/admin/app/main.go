package main

import (
    "github.com/gofiber/fiber/v2"
    "github.com/streadway/amqp"
    "log"
)

func main() {
    app := fiber.New()

    conn, err := amqp.Dial("amqp://guest:guest@rabbitmq:5672/")
    if err != nil {
        log.Fatalf("Failed to connect to RabbitMQ: %v", err)
    }
    defer conn.Close()

    channel, err := conn.Channel()
    if err != nil {
        log.Fatalf("Failed to open a channel: %v", err)
    }
    defer channel.Close()

    app.Post("/send-event", func(c *fiber.Ctx) error {
        body := c.FormValue("event")
        err = channel.Publish(
            "",    // exchange
            "queue_name", // routing key
            false, // mandatory
            false, // immediate
            amqp.Publishing{
                ContentType: "text/plain",
                Body:        []byte(body),
            },
        )
        if err != nil {
            return c.Status(500).SendString("Failed to publish a message")
        }
        return c.SendString("Message sent")
    })

    app.Static("/", "./frontend")
    log.Fatal(app.Listen(":8080"))
}

