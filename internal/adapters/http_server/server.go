package httpserver

import (
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
	"net/http"
	"time"
)

type Server struct{ mux *chi.Mux }

func New() *Server {
	m := chi.NewRouter()

	// ✅ All middlewares go here (before any routes are added)
	m.Use(chimw.RealIP)
	m.Use(chimw.RequestID)
	m.Use(chimw.Recoverer)           // chi's built-in recover
	m.Use(Timeout(15 * time.Second)) // timeout wrapper
	m.Use(Metrics)
	m.Use(Logger(log.Logger))

	return &Server{mux: m}
}

func (s *Server) Mux() http.Handler { return s.mux }

// Mount attaches any extra handler (e.g., /metrics) to the router.
func (s *Server) Mount(path string, h http.Handler) {
	s.mux.Handle(path, h)
}
