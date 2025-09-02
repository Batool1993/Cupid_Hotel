//go:build integration || !unit

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"cupid_hotel/internal/domain"
	mysqlrepo "cupid_hotel/internal/storage/mysql"
)

// ---------- helpers ----------
func pstr(s string) *string     { return &s }
func pint(i int) *int           { return &i }
func pfloat(f float64) *float64 { return &f }

func mustEnv(t *testing.T, k string) string {
	t.Helper()
	v := os.Getenv(k)
	if v == "" {
		t.Fatalf("%s not set; export it (e.g. MIGRATIONS_DIR=/path/to/sql)", k)
	}
	return v
}

func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	dir := mustEnv(t, "MIGRATIONS_DIR")

	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		t.Fatalf("MIGRATIONS_DIR=%s is not a directory or missing", dir)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var files []string
	for _, e := range ents {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		t.Fatalf("no .sql files in %s", dir)
	}
	sort.Strings(files)
	for _, f := range files {
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("exec %s: %v", f, err)
		}
	}
}

// ---------- tiny HTTP around repo (keeps wiring simple) ----------
type testAPI struct{ repo *mysqlrepo.Repo }

func (a *testAPI) hotel(w http.ResponseWriter, r *http.Request) {
	// Expect /v1/hotels/{id}
	idStr := strings.TrimPrefix(r.URL.Path, "/v1/hotels/")
	id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	hv, err := a.repo.GetHotel(r.Context(), id, lang)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	resp := struct {
		ID   int64   `json:"id"`
		Lang string  `json:"lang"`
		Name *string `json:"name"`
	}{
		ID:   hv.ID,
		Lang: lang,
		Name: hv.Name,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ---------- the test ----------
func TestHTTP_EndToEnd_Hotel_FR(t *testing.T) {
	// Start isolated MySQL container
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("dockertest: %v", err)
	}
	runOpts := &dockertest.RunOptions{
		Repository: "mysql",
		Tag:        "8.0.36",
		Env: []string{
			"MYSQL_ROOT_PASSWORD=root",
			"MYSQL_DATABASE=cupid",
		},
	}
	resource, err := pool.RunWithOptions(runOpts, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Fatalf("run mysql: %v", err)
	}
	t.Cleanup(func() { _ = pool.Purge(resource) })

	hostPort := resource.GetPort("3306/tcp")
	dsn := fmt.Sprintf("root:%s@tcp(127.0.0.1:%s)/%s?parseTime=true&multiStatements=true&charset=utf8mb4,utf8&loc=UTC",
		"root", hostPort, "cupid")

	var db *sql.DB
	if err := pool.Retry(func() error {
		var e error
		db, e = sql.Open("mysql", dsn)
		if e != nil {
			return e
		}
		return db.Ping()
	}); err != nil {
		t.Fatalf("connect mysql: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Apply your real migrations
	applyMigrations(t, db)

	repo := mysqlrepo.New(db)
	ctx := context.Background()

	// Seed with valid JSON blobs
	propID := int64(22002)
	h := domain.Hotel{
		ID:         propID,
		BrandID:    nil,
		Stars:      pint(4),
		Lat:        pfloat(41.0),
		Lon:        pfloat(29.0),
		Country:    pstr("TR"),
		City:       pstr("Istanbul"),
		AddressRaw: pstr("E2E Street"),
		Amenities:  []string{},
		Images:     []string{},
		RawJSON:    []byte(`{}`),
	}
	if err := repo.UpsertProperty(ctx, h); err != nil {
		t.Fatalf("UpsertProperty: %v", err)
	}
	if err := repo.UpsertI18n(ctx, domain.HotelI18n{
		PropertyID:  propID,
		Lang:        "fr",
		Name:        pstr("Hôtel E2E"),
		Description: pstr("Desc"),
		Policies:    pstr("Pol"),
		Address:     pstr("Adresse"),
		ExtrasJSON:  []byte(`{}`),
	}); err != nil {
		t.Fatalf("UpsertI18n: %v", err)
	}

	// Spin up minimal HTTP server exposing the one route we need
	api := &testAPI{repo: repo}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/hotels/", api.hotel)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Hit the endpoint
	res, err := http.Get(fmt.Sprintf("%s/v1/hotels/%d?lang=fr", ts.URL, propID))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}

	var body struct {
		ID   int64   `json:"id"`
		Lang string  `json:"lang"`
		Name *string `json:"name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != propID || body.Lang != "fr" || body.Name == nil || *body.Name != "Hôtel E2E" {
		t.Fatalf("unexpected body: %+v", body)
	}
}
