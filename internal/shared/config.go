package shared

import (
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	MetricsAddr string
	MySQLDSN    string
	RedisAddr   string
	RedisDB     int
	RedisPass   string
	CupidBase   string
	CupidKey    string
	Workers     int
	ReviewCount int
	CacheTTL    time.Duration
}

func Load() Config {
	atoi := func(k string, def int) int {
		if v := os.Getenv(k); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
		return def
	}
	c := Config{
		AppEnv:      env("APP_ENV", "prod"),
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		MetricsAddr: env("METRICS_ADDR", ":9100"),
		MySQLDSN:    env("MYSQL_DSN", "root:root@tcp(localhost:3306)/cupid?parseTime=true&charset=utf8mb4,utf8&loc=UTC"),
		RedisAddr:   env("REDIS_ADDR", "localhost:6379"),
		RedisPass:   env("REDIS_PASSWORD", ""),
		CupidBase:   env("CUPID_BASE_URL", "https://content-api.cupid.travel/v3.0"),
		CupidKey:    env("CUPID_API_KEY", ""),
		Workers:     atoi("INGEST_WORKERS", 8),
		ReviewCount: atoi("INGEST_REVIEW_COUNT", 100),
		CacheTTL:    time.Duration(atoi("CACHE_TTL_SECONDS", 900)) * time.Second,
	}
	if c.CupidKey == "" {
		log.Warn().Msg("CUPID_API_KEY is empty")
	}
	return c
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
