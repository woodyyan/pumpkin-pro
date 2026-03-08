package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status": "online", "service": "Pumpkin Backend Engine"}`)
	})

	fmt.Println("🚀 Backend Engine is running on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
