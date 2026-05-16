package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	go func() {
		log.Fatal(http.ListenAndServe("127.0.0.1:8088", mux))
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for now := range ticker.C {
		fmt.Println("heartbeat", now.Format(time.RFC3339))
	}
}
