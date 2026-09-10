package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardServerRoutes(t *testing.T) {
	srv := NewServer(4242)

	t.Run("GET / returns 200 HTML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		srv.handleDashboard(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rec.Code)
		}
		if contentType := rec.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
			t.Errorf("Expected text/html content-type, got %s", contentType)
		}
	})

	t.Run("GET /api/runs returns 200 JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
		rec := httptest.NewRecorder()

		srv.handleListRuns(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("SSE broadcast delivers event to subscriber", func(t *testing.T) {
		broker := NewEventBroker()
		ch := broker.Subscribe()
		defer broker.Unsubscribe(ch)

		msg := []byte(`{"event":"stage_complete","vus":100}`)
		broker.Broadcast(msg)

		select {
		case received := <-ch:
			if string(received) != string(msg) {
				t.Errorf("Expected message %s, got %s", string(msg), string(received))
			}
		default:
			t.Errorf("Expected to receive broadcast message")
		}
	})
}
