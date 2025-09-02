package observability

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// NewLogger returns a zerolog Logger.
// APP_ENV=dev (or development) uses a human-friendly console writer.
func NewLogger(env string) zerolog.Logger {
	l := zerolog.New(os.Stdout).With().Timestamp().Logger()
	if env == "dev" || env == "development" {
		l = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
			With().Timestamp().Logger()
	}
	return l
}
