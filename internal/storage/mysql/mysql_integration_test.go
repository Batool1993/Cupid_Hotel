//go:build integration || !unit

package mysql_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"cupid_hotel/internal/domain"
	mysqlrepo "cupid_hotel/internal/storage/mysql"
)

// ---------- small helpers ----------
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

// ---------- the test ----------
func TestRepo_MySQL_UpsertAndQuery(t *testing.T) {
	// Start isolated MySQL; let Docker pick a free host port.
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

	applyMigrations(t, db)

	repo := mysqlrepo.New(db)
	ctx := context.Background()

	// Arrange — seed with valid JSON blobs
	h := domain.Hotel{
		ID:         10001,
		BrandID:    nil,
		Stars:      pint(5),
		Lat:        pfloat(41.02),
		Lon:        pfloat(29.01),
		Country:    pstr("TR"),
		City:       pstr("Istanbul"),
		AddressRaw: pstr("Somewhere 1"),
		Amenities:  []string{}, // marshals to "[]"
		Images:     []string{}, // marshals to "[]"
		RawJSON:    []byte(`{}`),
	}
	if err := repo.UpsertProperty(ctx, h); err != nil {
		t.Fatalf("UpsertProperty: %v", err)
	}

	i18 := domain.HotelI18n{
		PropertyID:  10001,
		Lang:        "fr",
		Name:        pstr("Hôtel Test"),
		Description: pstr("Desc"),
		Policies:    pstr("Pol"),
		Address:     pstr("Adresse"),
		ExtrasJSON:  []byte(`{}`),
	}
	if err := repo.UpsertI18n(ctx, i18); err != nil {
		t.Fatalf("UpsertI18n: %v", err)
	}

	r1 := domain.Review{
		PropertyID:  10001,
		SourceID:    pstr("s-1"),
		Author:      pstr("Ana"),
		Rating:      pfloat(9.0),
		Lang:        pstr("fr"),
		Title:       pstr("Bien"),
		Text:        pstr("…"),
		Source:      pstr("cupid"),
		AspectsJSON: []byte(`[]`), // safe for TEXT/JSON column, not ""
		RawJSON:     []byte(`{}`),
	}
	r2 := domain.Review{
		PropertyID:  10001,
		SourceID:    pstr("s-2"),
		Author:      pstr("Bob"),
		Rating:      pfloat(7.5),
		Lang:        pstr("fr"),
		Title:       pstr("Ok"),
		Text:        pstr("…"),
		Source:      pstr("cupid"),
		AspectsJSON: []byte(`[]`),
		RawJSON:     []byte(`{}`),
	}
	if err := repo.UpsertReviews(ctx, []domain.Review{r1, r2}); err != nil {
		t.Fatalf("UpsertReviews: %v", err)
	}

	// Assert
	hv, err := repo.GetHotel(ctx, 10001, "fr")
	if err != nil {
		t.Fatalf("GetHotel: %v", err)
	}
	if hv.ID != 10001 || hv.Name == nil || *hv.Name != "Hôtel Test" {
		t.Fatalf("unexpected hotel view: %+v", hv)
	}

	// Optional: small sleep to let CURRENT_TIMESTAMP settle in container clocks
	time.Sleep(50 * time.Millisecond)
}
