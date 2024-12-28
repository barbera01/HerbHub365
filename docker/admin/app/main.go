package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/streadway/amqp"
)

// Message represents the structure of the event sent to RabbitMQ
type Message struct {
	Event string `json:"event"`
}

// getRabbitMQConnection initializes a connection to RabbitMQ
func getRabbitMQConnection() (*amqp.Connection, error) {
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}
	return amqp.Dial(rabbitmqURL)
}

func main() {
	// Initialize router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Connect to RabbitMQ
	conn, err := getRabbitMQConnection()
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer channel.Close()

	// Define HTTP endpoints
	r.Post("/send-event", func(w http.ResponseWriter, r *http.Request) {
		var msg Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		err = channel.Publish(
			"",             // exchange
			"queue_name",   // routing key
			false,          // mandatory
			false,          // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte(msg.Event),
			},
		)
		if err != nil {
			http.Error(w, "Failed to publish message", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Message sent"))
	})

	// Serve static files for the frontend
	r.Handle("/*", http.FileServer(http.Dir("./frontend")))

	// Start the HTTP server
	log.Println("Server running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}



