package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "api-gateway ok")
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"healthy"}`)
	})

	fmt.Printf("api-gateway listening on :%s\n", port)
	http.ListenAndServe(":"+port, nil)
}
