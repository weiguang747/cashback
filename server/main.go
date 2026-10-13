package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	// Read the port from the environment so Docker/hosting platforms can set it.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Listen on all interfaces (":8080"), not "localhost:8080" —
	// inside a container, localhost is unreachable from your Mac.
	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
