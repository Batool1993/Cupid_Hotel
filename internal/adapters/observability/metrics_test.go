package observability_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cupid_hotel/internal/adapters/observability"
)

func TestMetricsRegistryAndHandler(t *testing.T) {
	reg := observability.InitRegistry()

	// record one sample so counters are non-zero
	observability.ObserveHTTP("/test", "GET", 200, 12*time.Millisecond)

	mh := observability.MetricsHandler(reg)
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	mh.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("metrics status: %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	out := string(body)
	if !strings.Contains(out, "cupid_http_requests_total") {
		t.Fatalf("expected cupid_http_requests_total in output")
	}
}
