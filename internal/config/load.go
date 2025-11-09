package config

import (
	"fmt"

	"github.com/caarlos0/env/v6"
)

// Load читает переменные окружения и заполняет Config.
func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// MustLoad удобен там, где ошибки конфигурации должны аварийно останавливать процесс.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}
