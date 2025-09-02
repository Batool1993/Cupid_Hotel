package cupid_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cupid_hotel/internal/adapters/cupid"
)

func TestClient_GetProperty_RetriesThenSuccess(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch atomic.AddInt32(&hits, 1) {
		case 1, 2:
			// two transient failures
			w.WriteHeader(500)
		default:
			w.WriteHeader(200)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 123.0})
		}
	}))
	defer ts.Close()

	cl, err := cupid.New(ts.URL, "test-key", 100) // high RPS for tests
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := cl.GetProperty(ctx, 123)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	id, ok := got["id"].(float64)
	if !ok || int(id) != 123 {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if atomic.LoadInt32(&hits) < 3 {
		t.Fatalf("expected at least 3 calls due to retries, got %d", hits)
	}
}

func TestClient_GetProperty_404(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()

	cl, err := cupid.New(ts.URL, "test-key", 100)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = cl.GetProperty(ctx, 1)
	if err == nil {
		t.Fatalf("expected error for 404")
	}
}
