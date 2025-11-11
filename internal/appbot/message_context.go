package appbot

import (
	"context"
	"fmt"
	"strings"

	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
)

// MessageContext предоставляет помощники для обработчиков сообщений.
type MessageContext struct {
	update  *schemes.MessageCreatedUpdate
	service *Service
	log     zerolog.Logger
}

func newMessageContext(service *Service, update *schemes.MessageCreatedUpdate) *MessageContext {
	logger := service.log.With().
		Str("update_type", string(update.GetUpdateType())).
		Int64("user_id", update.GetUserID()).
		Int64("chat_id", update.GetChatID()).
		Logger()

	return &MessageContext{
		update:  update,
		service: service,
		log:     logger,
	}
}

// Logger возвращает логгер с полями пользователя и чата.
func (mc *MessageContext) Logger() zerolog.Logger {
	return mc.log
}

// Update отдаёт исходный апдейт MAX.
func (mc *MessageContext) Update() *schemes.MessageCreatedUpdate {
	return mc.update
}

// API возвращает низкоуровневый клиент MAX для кастомной логики.
func (mc *MessageContext) API() *maxbot.Api {
	return mc.service.api
}

// Text возвращает текст сообщения без лишних пробелов.
func (mc *MessageContext) Text() string {
	return strings.TrimSpace(mc.update.GetText())
}

// Command выдаёт нормализованное имя команды без префикса «/».
func (mc *MessageContext) Command() string {
	text := mc.Text()
	if text == "" || !strings.HasPrefix(text, "/") {
		return ""
	}

	first := strings.Fields(text)
	if len(first) == 0 {
		return ""
	}

	return normalizeCommand(first[0])
}

// Args возвращает аргументы сообщения, разделённые пробелами.
func (mc *MessageContext) Args() []string {
	text := mc.Text()
	parts := strings.Fields(text)
	if len(parts) <= 1 {
		return nil
	}
	return parts[1:]
}

// SenderID возвращает идентификатор автора сообщения.
func (mc *MessageContext) SenderID() int64 {
	return mc.update.GetUserID()
}

// SenderName возвращает отображаемое имя автора.
func (mc *MessageContext) SenderName() string {
	return mc.update.Message.Sender.Name
}

// ChatID возвращает идентификатор чата, если он есть.
func (mc *MessageContext) ChatID() int64 {
	return mc.update.GetChatID()
}

// ReplyText отправляет текстовый ответ в исходный чат или пользователю.
func (mc *MessageContext) ReplyText(ctx context.Context, text string) error {
	return mc.service.sendText(ctx, text, mc.SenderID(), mc.ChatID())
}

// Replyf отправляет отформатированный ответ.
func (mc *MessageContext) Replyf(ctx context.Context, format string, args ...interface{}) error {
	return mc.ReplyText(ctx, fmt.Sprintf(format, args...))
}

// SessionState возвращает текущее состояние пользователя, если оно есть.
func (mc *MessageContext) SessionState() (SessionState, bool) {
	return mc.service.SessionState(mc.SenderID())
}

// SetSessionState сохраняет шаг сессии для пользователя.
func (mc *MessageContext) SetSessionState(state SessionState) {
	mc.service.SetSessionState(mc.SenderID(), state)
}

// ClearSessionState очищает состояние пользователя.
func (mc *MessageContext) ClearSessionState() {
	mc.service.ClearSessionState(mc.SenderID())
}

// Service возвращает bot service для расширенных операций отправки сообщений.
func (mc *MessageContext) Service() *Service {
	return mc.service
}
