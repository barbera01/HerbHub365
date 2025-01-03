package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Example function to trigger
func triggerFunction() {
	fmt.Println("Button was clicked! Function triggered.")
}

func main() {
	// Create a new router
	r := chi.NewRouter()

	// Add middleware
	r.Use(middleware.Logger)    // Logs requests
	r.Use(middleware.Recoverer) // Recovers from panics

	// Define routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})

	r.Post("/trigger", func(w http.ResponseWriter, r *http.Request) {
		// Trigger your Go function
		triggerFunction()

		// Respond to the client
		w.Write([]byte("Function triggered successfully!"))
	})

	// Start the server
	http.ListenAndServe(":8080", r)
}

