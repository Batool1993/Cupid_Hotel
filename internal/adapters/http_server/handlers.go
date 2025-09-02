// internal/adapters/http_server/handlers.go
package httpserver

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"cupid_hotel/internal/app"
	"cupid_hotel/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

type Handlers struct{ Q *app.QueryService }

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func (s *Server) MountHandlers(h *Handlers) {
	s.mux.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	s.mux.Get("/v1/hotels/{id}", h.getHotel)
	s.mux.Get("/v1/hotels/{id}/reviews", h.listReviews)
}

func selectLang(al string) string {
	s := strings.ToLower(al)
	if strings.HasPrefix(s, "fr") {
		return "fr"
	}
	if strings.HasPrefix(s, "es") {
		return "es"
	}
	return "en"
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(problem{Type: "about:blank", Title: title, Status: status, Detail: detail}); err != nil {
		log.Error().Err(err).Msg("write JSON problem response failed")
	}
}

// calcETagAndBody marshals once and hashes once, returning both ETag and body.
func calcETagAndBody(v any) (string, []byte) {
	body, err := json.Marshal(v)
	if err != nil {
		// Log but don't fail the whole response; return empty ETag and best-effort body.
		log.Error().Err(err).Msg("failed to marshal object for ETag/body")
		return "", nil
	}
	sum := sha1.Sum(body)
	etag := `W/"` + hex.EncodeToString(sum[:]) + `"`
	return etag, body
}

func (h *Handlers) getHotel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = selectLang(r.Header.Get("Accept-Language"))
	}
	resp, err := h.Q.GetHotel(r.Context(), id, lang)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "Not Found", "hotel not found")
		return
	}

	etag, body := calcETagAndBody(resp)
	// If client already has this version, short-circuit.
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.Header().Set("ETag", etag) // include ETag on 304
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Language", resp.Language)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		log.Error().Err(err).Msg("failed to write getHotel body")
	}
}

func (h *Handlers) listReviews(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid ID", "id must be a number")
		return
	}

	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		l, err := strconv.Atoi(ls)
		if err != nil || l <= 0 || l > 200 {
			writeProblem(w, http.StatusBadRequest, "Invalid limit", "limit must be an integer between 1 and 200")
			return
		}
		limit = l
	}

	// Newest first; aligns with DB index on (property_id, created_at, id)
	page := domain.PageQuery{Limit: limit, Cursor: nil, Sort: "-created_at"}
	out, err := h.Q.ListReviews(r.Context(), id, page)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "Not Found", "reviews not found")
		return
	}

	etag, body := calcETagAndBody(out)
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		log.Error().Err(err).Msg("failed to write listReviews body")
	}
}
