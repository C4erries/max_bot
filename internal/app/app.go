package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/c4erries/max_bot/internal/appbot"
	"github.com/c4erries/max_bot/internal/config"
	"github.com/rs/zerolog"
)

// Module — долгоживущий компонент, которым управляет приложение.
type Module interface {
	Run(context.Context) error
}

// Application координирует все модули (бот и дополнительные сервисы).
type Application struct {
	modules []moduleEntry
	log     zerolog.Logger
}

type moduleEntry struct {
	name   string
	module Module
}

// New собирает приложение, регистрирует бота и базовые обработчики.
func New(bot *appbot.Service, log zerolog.Logger, cfg *config.Config) *Application {
	if bot == nil {
		panic("app: bot service is nil")
	}
	if cfg == nil {
		panic("app: config is nil")
	}

	applications, err := newApplicationCoordinator(cfg.Backend.APIBaseURL, log)
	if err != nil {
		panic(fmt.Sprintf("app: %v", err))
	}

	registerDefaultBotHandlers(bot, applications)

	return &Application{
		log: log.With().Str("component", "app").Logger(),
		modules: []moduleEntry{
			{name: "bot", module: bot},
		},
	}
}

// RegisterModule позволяет добавить дополнительные фоновые воркеры (например HTTP-сервер).
func (a *Application) RegisterModule(name string, module Module) {
	if name == "" || module == nil {
		return
	}
	a.modules = append(a.modules, moduleEntry{name: name, module: module})
}

// Run запускает все модули и блокируется до отмены контекста или ошибки.
func (a *Application) Run(ctx context.Context) error {
	if len(a.modules) == 0 {
		return errors.New("app: no modules registered")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(a.modules))
	var wg sync.WaitGroup

	for _, module := range a.modules {
		module := module
		wg.Add(1)

		go func() {
			defer wg.Done()
			a.log.Info().Str("module", module.name).Msg("module started")

			if err := module.module.Run(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					a.log.Debug().Str("module", module.name).Msg("module canceled")
					return
				}
				errCh <- fmt.Errorf("%s: %w", module.name, err)
				return
			}

			a.log.Info().Str("module", module.name).Msg("module stopped")
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	for {
		select {
		case err := <-errCh:
			if err != nil {
				cancel()
				<-done
				return err
			}
		case <-ctx.Done():
			cancel()
			<-done
			return ctx.Err()
		case <-done:
			return nil
		}
	}
}
