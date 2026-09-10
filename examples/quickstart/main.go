package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Item struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	itemsLock sync.RWMutex
	itemsList = []Item{
		{ID: "item-1", Name: "Standard Widget", CreatedAt: time.Now()},
		{ID: "item-2", Name: "Enterprise Gadget", CreatedAt: time.Now()},
	}
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Health Check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "quickstart-api"})
	})

	// User Authentication
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Simulate slight crypto password verification delay
		time.Sleep(5 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":   "mock-jwt-token-quickstart-capacitylab-user",
			"message": "Authentication successful",
		})
	})

	// List Items (Protected or Public)
	mux.HandleFunc("GET /api/v1/items", func(w http.ResponseWriter, r *http.Request) {
		// Simulate database lookup latency with slight variance
		jitter := time.Duration(3+rand.Intn(7)) * time.Millisecond
		time.Sleep(jitter)

		itemsLock.RLock()
		defer itemsLock.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": itemsList,
			"count": len(itemsList),
		})
	})

	// Create Item
	mux.HandleFunc("POST /api/v1/items", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var payload struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Name == "" {
			payload.Name = fmt.Sprintf("Auto-Item-%d", time.Now().UnixNano()%1000)
		}

		newItem := Item{
			ID:        fmt.Sprintf("item-%d", time.Now().UnixNano()),
			Name:      payload.Name,
			CreatedAt: time.Now(),
		}

		itemsLock.Lock()
		itemsList = append(itemsList, newItem)
		if len(itemsList) > 1000 {
			itemsList = itemsList[500:] // Cap in-memory slice
		}
		itemsLock.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(newItem)
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("🚀 Quickstart API running on http://0.0.0.0:%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
