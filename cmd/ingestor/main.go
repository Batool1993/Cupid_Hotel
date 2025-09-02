package main

import (
	"context"
	"cupid_hotel/internal/adapters/observability"
	redisad "cupid_hotel/internal/adapters/redis"
	"database/sql"
	"sync"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/semaphore"

	"cupid_hotel/internal/adapters/cupid"
	"cupid_hotel/internal/app"
	"cupid_hotel/internal/shared"
	mysqlrepo "cupid_hotel/internal/storage/mysql"
)

func main() {
	ctx := context.Background()
	cfg := shared.Load()

	// 1) initialize global logger (console in dev, JSON otherwise)
	log.Logger = observability.NewLogger(cfg.AppEnv)

	log.Info().
		Str("base", cfg.CupidBase).
		Int("workers", cfg.Workers).
		Int("reviews", cfg.ReviewCount).
		Msg("ingestor starting")

	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("sql.Open failed")
	}
	if err := db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("db.Ping failed")
	}
	log.Info().Msg("db ping ok")

	repo := mysqlrepo.New(db)

	client, err := cupid.New(cfg.CupidBase, cfg.CupidKey, 5)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize Cupid client")
	}
	cache := redisad.New(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)
	ing := app.NewIngestionService(client, repo, cache)
	sem := semaphore.NewWeighted(int64(cfg.Workers))
	var wg sync.WaitGroup

	for _, id := range shared.PropertyIDs {
		id := id

		// acquire before launching the goroutine; release inside it
		if err := sem.Acquire(ctx, int64(1)); err != nil {
			log.Fatal().Err(err).Msg("semaphore acquire failed")
		}

		wg.Add(1)
		go func(hotelID int64) {
			defer wg.Done()
			defer sem.Release(int64(1))

			if err := ing.IngestHotel(ctx, hotelID, cfg.ReviewCount); err != nil {
				log.Warn().Int64("id", hotelID).Err(err).Msg("ingest failed")
				return
			}
			log.Info().Int64("id", hotelID).Msg("ingest ok")
		}(id)
	}

	wg.Wait()
	log.Info().Msg("ingestion completed")
}
