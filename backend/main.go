package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getQuantURL() string {
	quantURL := os.Getenv("QUANT_SERVICE_URL")
	if quantURL == "" {
		quantURL = "http://localhost:8000"
	}
	return strings.TrimRight(quantURL, "/")
}

func proxyToQuant(w http.ResponseWriter, r *http.Request, targetPath string) {
	targetURL := getQuantURL() + targetPath

	var bodyReader io.Reader
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
		bodyReader = bytes.NewBuffer(body)
	}

	req, err := http.NewRequest(r.Method, targetURL, bodyReader)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if accept := r.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}

	log.Printf("Forwarding %s request to %s\n", r.Method, targetURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Error calling quant service: %v", err)
		http.Error(w, "Failed to connect to quant engine", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error copying response: %v", err)
	}
}

func handleBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	proxyToQuant(w, r, "/api/backtest")
}

func handleBacktestOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
		return
	}
	proxyToQuant(w, r, "/api/backtest/options")
}

func handleStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Only GET and POST methods are allowed", http.StatusMethodNotAllowed)
		return
	}
	proxyToQuant(w, r, "/api/strategies")
}

func handleActiveStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
		return
	}
	proxyToQuant(w, r, "/api/strategies/active")
}

func handleStrategySubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/strategies/")
	if suffix == "" || suffix == r.URL.Path {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		http.Error(w, "Only GET and PUT methods are allowed", http.StatusMethodNotAllowed)
		return
	}

	proxyToQuant(w, r, "/api/strategies/"+suffix)
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status": "online", "service": "Pumpkin Go Backend"}`)
	})

	mux.HandleFunc("/api/backtest", handleBacktest)
	mux.HandleFunc("/api/backtest/options", handleBacktestOptions)
	mux.HandleFunc("/api/strategies", handleStrategies)
	mux.HandleFunc("/api/strategies/active", handleActiveStrategies)
	mux.HandleFunc("/api/strategies/", handleStrategySubroutes)

	handler := corsMiddleware(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Pumpkin Go Backend is running on port %s...", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
