package main

import (
	"database/sql"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"

	server "cupid_hotel/internal/adapters/http_server"
	"cupid_hotel/internal/adapters/observability"
	redisad "cupid_hotel/internal/adapters/redis"
	"cupid_hotel/internal/app"
	"cupid_hotel/internal/shared"
	mysqlrepo "cupid_hotel/internal/storage/mysql"
)

func main() {
	cfg := shared.Load()

	// set global logger (console in dev, JSON otherwise)
	log.Logger = observability.NewLogger(cfg.AppEnv)

	observability.Serve()

	// db
	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("sql.Open failed")
	}
	if err := db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("db.Ping failed")
	}
	log.Info().Msg("database connection ok")

	// deps
	repo := mysqlrepo.New(db)
	cache := redisad.New(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)
	q := app.NewQueryService(repo, cache, cfg.CacheTTL)

	// http
	srv := server.New()
	reg := observability.InitRegistry()
	srv.Mount("/metrics", observability.MetricsHandler(reg))
	srv.MountHandlers(&server.Handlers{Q: q})

	log.Info().Str("addr", cfg.HTTPAddr).Msg("API listening")
	httpSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Mux()}

	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("http server failed")
	}
}
