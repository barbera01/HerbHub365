package main

import "net/http"

// HomeHandler serves the home page
func HomeHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("<html><body><h1>Welcome to the Watering System</h1></body></html>"))
}
