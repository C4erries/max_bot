package appbot

import (
	"context"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
)

// BotStartedContext инкапсулирует событие нажатия пользователем кнопки "Старт".
type BotStartedContext struct {
	update  *schemes.BotStartedUpdate
	service *Service
	log     zerolog.Logger
}

func newBotStartedContext(service *Service, update *schemes.BotStartedUpdate) *BotStartedContext {
	logger := service.log.With().
		Str("update_type", string(update.GetUpdateType())).
		Int64("user_id", update.GetUserID()).
		Int64("chat_id", update.GetChatID()).
		Logger()

	return &BotStartedContext{
		update:  update,
		service: service,
		log:     logger,
	}
}

// Logger возвращает контекстный логгер.
func (bc *BotStartedContext) Logger() zerolog.Logger {
	return bc.log
}

// ChatID возвращает идентификатор чата, где пользователь нажал Start.
func (bc *BotStartedContext) ChatID() int64 {
	return bc.update.GetChatID()
}

// UserID возвращает идентификатор пользователя.
func (bc *BotStartedContext) UserID() int64 {
	return bc.update.GetUserID()
}

// ReplyText отправляет текстовое сообщение в диалог.
func (bc *BotStartedContext) ReplyText(ctx context.Context, text string) error {
	return bc.service.sendText(ctx, text, bc.UserID(), bc.ChatID())
}

// Service предоставляет доступ к bot-сервису.
func (bc *BotStartedContext) Service() *Service {
	return bc.service
}

// Update возвращает исходное событие.
func (bc *BotStartedContext) Update() *schemes.BotStartedUpdate {
	return bc.update
}
