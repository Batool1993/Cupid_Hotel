package httpserver

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"cupid_hotel/internal/adapters/observability"
)

func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return http.TimeoutHandler(next, d, "timeout") }
}

// ---- status-recording ResponseWriter ----

type srw struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *srw) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *srw) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *srw) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// ---- Metrics middleware ----

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &srw{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = r.URL.Path
		}
		observability.ObserveHTTP(route, r.Method, sw.Status(), time.Since(start))
	})
}

// ---- Structured logging middleware ----

func Logger(l zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &srw{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = r.URL.Path
			}
			l.Info().
				Str("route", route).
				Str("method", r.Method).
				Int("status", sw.Status()).
				Dur("duration", time.Since(start)).
				Str("remote", remoteIP(r)).
				Str("ua", r.UserAgent()).
				Msg("http_request")
		})
	}
}

// Picks first X-Forwarded-For IP, else X-Real-IP, else RemoteAddr host.
func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
