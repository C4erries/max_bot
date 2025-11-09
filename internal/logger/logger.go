package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New создаёт zerolog-логгер, настроенный на вывод в консоль.
func New(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	logger := zerolog.New(output).With().Timestamp().Logger()
	return logger.Level(lvl)
}
