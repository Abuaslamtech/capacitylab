package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/history"
	"github.com/Abuaslamtech/capacitylab/internal/report"
)

// EventBroker manages SSE client connections
type EventBroker struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
}

var GlobalBroker = NewEventBroker()

func NewEventBroker() *EventBroker {
	return &EventBroker{
		clients: make(map[chan []byte]bool),
	}
}

func (b *EventBroker) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 16)
	b.clients[ch] = true
	return ch
}

func (b *EventBroker) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
}

func (b *EventBroker) Broadcast(msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// PublishLiveEvent broadcasts a live benchmark event to any connected web dashboard.
func PublishLiveEvent(eventType string, payload any) {
	data, err := json.Marshal(map[string]any{
		"type":      eventType,
		"timestamp": time.Now().Format(time.RFC3339),
		"payload":   payload,
	})
	if err == nil {
		GlobalBroker.Broadcast(data)
	}
}

// Server serves the local web dashboard and streaming API
type Server struct {
	port   int
	server *http.Server
	broker *EventBroker
}

// NewServer initializes a dashboard HTTP server
func NewServer(port int) *Server {
	if port <= 0 {
		port = 4242
	}
	return &Server{
		port:   port,
		broker: GlobalBroker,
	}
}

// Start launches the HTTP server and blocks until ctx is cancelled
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// 1. Dashboard View
	mux.HandleFunc("/", s.handleDashboard)

	// 2. REST APIs
	mux.HandleFunc("/api/runs", s.handleListRuns)
	mux.HandleFunc("/api/runs/", s.handleGetRun)
	mux.HandleFunc("/api/events", s.handlePostEvent)

	// 3. Real-Time SSE Stream
	mux.HandleFunc("/api/stream", s.handleStream)

	addr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()

	return s.server.ListenAndServe()
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	runs, err := history.ListRuns()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load runs: %v", err), http.StatusInternalServerError)
		return
	}

	var summaries []report.DashboardRunSummary
	for _, run := range runs {
		summaries = append(summaries, report.DashboardRunSummary{
			ID:             run.ID,
			Date:           run.Timestamp.Format("Jan 02 15:04"),
			AppName:        run.AppName,
			TargetURL:      run.TargetURL,
			SustainableVUs: run.SustainableVUs,
			MaxObservedVUs: run.MaxObservedVUs,
			PeakRPS:        run.PeakRPS,
			P95Ms:          run.P95LatencyMs,
			Bottleneck:     run.PrimaryBottleneck,
			Fix:            run.BottleneckFix,
		})
	}

	htmlBytes, err := report.RenderDashboardHTML(summaries)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render dashboard: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(htmlBytes)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := history.ListRuns()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	if runID == "" {
		http.Error(w, "missing run ID", http.StatusBadRequest)
		return
	}

	runs, err := history.ListRuns()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, run := range runs {
		if run.ID == runID {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(run)
			return
		}
	}

	http.NotFound(w, r)
}

func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	s.broker.Broadcast(body)
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"broadcasted"}`))
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientCh := s.broker.Subscribe()
	defer s.broker.Unsubscribe(clientCh)

	// Send initial ping
	_, _ = fmt.Fprintf(w, "event: connected\ndata: %s\n\n", strconv.Quote("Connected to CapacityLab Telemetry Stream"))
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-clientCh:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "event: telemetry\ndata: %s\n\n", string(msg))
			flusher.Flush()
		}
	}
}
