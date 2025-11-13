package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/c4erries/max_bot/internal/app"
	"github.com/c4erries/max_bot/internal/appbot"
	"github.com/c4erries/max_bot/internal/backend"
	"github.com/c4erries/max_bot/internal/config"
	"github.com/c4erries/max_bot/internal/httpserver"
	"github.com/c4erries/max_bot/internal/logger"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to load config: " + err.Error())
		os.Exit(1)
	}

	log := logger.New(cfg.Logger.Level)
	logger.RedirectStdLogger(log, cfg.Bot.Token)

	api, err := maxbot.New(cfg.Bot.Token)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init max api client")
	}

	bot := appbot.NewService(api, log)

	repo, err := backend.NewRepository(cfg.Backend.APIBaseURL, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init backend repository")
	}

	payments := repo.Payments()
	if payments == nil {
		log.Fatal().Msg("failed to init payments backend")
	}

	application := app.New(bot, log, repo)

	if addr := strings.TrimSpace(cfg.HTTP.Address); addr != "" {
		token := strings.TrimSpace(cfg.HTTP.BackendToken)
		if token == "" {
			log.Warn().Msg("HTTP backend token is empty; relying on docker network isolation")
		}
		notifier := app.NewNotifier(bot, payments)
		httpSrv := httpserver.New(addr, notifier, token, log)
		application.RegisterModule("http", httpSrv)
	}

	log.Info().Msg("max bot starting up")
	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("application stopped with error")
		os.Exit(1)
	}
	log.Info().Msg("max bot stopped gracefully")
}
