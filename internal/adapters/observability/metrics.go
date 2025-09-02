package observability

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "cupid", Name: "http_requests_total", Help: "HTTP requests."},
		[]string{"route", "method", "status"},
	)
	HTTPLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "cupid", Name: "http_request_duration_seconds",
			Help:    "HTTP request duration seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route", "method"},
	)
	ExternalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "cupid", Name: "external_requests_total", Help: "Outbound requests."},
		[]string{"service", "endpoint", "status"},
	)
	ExternalLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "cupid", Name: "external_request_duration_seconds",
			Help:    "Outbound request duration seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "endpoint"},
	)
	CacheEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "cupid", Name: "cache_events_total", Help: "Cache hits/misses/sets/dels."},
		[]string{"cache", "event"}, // event: hit|miss|set|del
	)
)

func Serve() {
	addr := os.Getenv("METRICS_ADDR")
	if addr == "" {
		return // disabled
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	go func() {
		srv := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		log.Info().Str("addr", addr).Msg("metrics server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("metrics server failed")
		}
	}()
}

func InitRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(HTTPRequests, HTTPLatency, ExternalRequests, ExternalLatency, CacheEvents)
	return reg
}

func MetricsHandler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

func ObserveHTTP(route, method string, status int, dur time.Duration) {
	HTTPRequests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
	HTTPLatency.WithLabelValues(route, method).Observe(dur.Seconds())
}

func ObserveExternal(service, endpoint string, status int, dur time.Duration) {
	ExternalRequests.WithLabelValues(service, endpoint, strconv.Itoa(status)).Inc()
	ExternalLatency.WithLabelValues(service, endpoint).Observe(dur.Seconds())
}

func ObserveCache(cache, event string) { // event: hit|miss|set|del
	CacheEvents.WithLabelValues(cache, event).Inc()
}

func LabelErr(err error) string {
	if err == nil {
		return "none"
	}
	return fmt.Sprintf("%T", err)
}
