package appbot

import (
	"context"
	"fmt"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
)

// CallbackContext содержит информацию о нажатии на инлайн-кнопку.
type CallbackContext struct {
	update  *schemes.MessageCallbackUpdate
	service *Service
	log     zerolog.Logger
}

func newCallbackContext(service *Service, update *schemes.MessageCallbackUpdate) *CallbackContext {
	logger := service.log.With().
		Str("update_type", string(update.GetUpdateType())).
		Int64("user_id", update.Callback.GetUserID()).
		Str("payload", update.Callback.Payload).
		Logger()

	return &CallbackContext{
		update:  update,
		service: service,
		log:     logger,
	}
}

// Logger возвращает scoped-логгер.
func (cc *CallbackContext) Logger() zerolog.Logger {
	return cc.log
}

// Update возвращает оригинальное событие.
func (cc *CallbackContext) Update() *schemes.MessageCallbackUpdate {
	return cc.update
}

// Payload возвращает полезную нагрузку кнопки.
func (cc *CallbackContext) Payload() string {
	return cc.update.Callback.Payload
}

// SenderID возвращает идентификатор пользователя.
func (cc *CallbackContext) SenderID() int64 {
	return cc.update.Callback.GetUserID()
}

// SenderName возвращает имя пользователя.
func (cc *CallbackContext) SenderName() string {
	return cc.update.Callback.User.Name
}

// ChatID пытается определить чат из исходного сообщения.
func (cc *CallbackContext) ChatID() int64 {
	if cc.update.Message != nil {
		return cc.update.Message.Recipient.ChatId
	}
	return 0
}

// ReplyText отправляет новое сообщение в диалог.
func (cc *CallbackContext) ReplyText(ctx context.Context, text string) error {
	return cc.service.sendText(ctx, text, cc.SenderID(), cc.ChatID())
}

// Answer отправляет ответ на callback, чтобы клиент перестал ожидать.
func (cc *CallbackContext) Answer(ctx context.Context, answer *schemes.CallbackAnswer) error {
	if answer == nil {
		return nil
	}
	if answer.Notification == "" && answer.Message == nil {
		return nil
	}
	if cc.service.api == nil || cc.service.api.Messages == nil {
		return fmt.Errorf("appbot: api client is nil")
	}
	_, err := cc.service.api.Messages.AnswerOnCallback(ctx, cc.update.Callback.CallbackID, answer)
	return err
}

// SessionState возвращает текущее состояние пользователя.
func (cc *CallbackContext) SessionState() (SessionState, bool) {
	return cc.service.SessionState(cc.SenderID())
}

// SetSessionState сохраняет состояние пользователя.
func (cc *CallbackContext) SetSessionState(state SessionState) {
	cc.service.SetSessionState(cc.SenderID(), state)
}

// ClearSessionState сбрасывает состояние пользователя.
func (cc *CallbackContext) ClearSessionState() {
	cc.service.ClearSessionState(cc.SenderID())
}

// Service возвращает ссылку на bot-service (нужна для отправки произвольных сообщений).
func (cc *CallbackContext) Service() *Service {
	return cc.service
}
