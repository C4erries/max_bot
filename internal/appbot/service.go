package appbot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
)

// UpdatesProvider определяет источник апдейтов Max API (long polling/webhook).
type UpdatesProvider interface {
	GetUpdates(context.Context) <-chan schemes.UpdateInterface
}

// MessageSender отправляет сообщения от имени бота.
type MessageSender interface {
	Send(context.Context, *maxbot.Message) (string, error)
}

// CommandHandler вызывается, когда пользователь отправляет подходящую слеш-команду.
type CommandHandler func(context.Context, *MessageContext) error

// MessageHandler обрабатывает сообщения, для которых не найдено ни одной команды.
type MessageHandler func(context.Context, *MessageContext) error

// CallbackHandler обрабатывает нажатия на кнопки инлайн-клавиатуры.
type CallbackHandler func(context.Context, *CallbackContext) error

// Command описывает обработчик, который можно зарегистрировать в сервисе бота.
type Command struct {
	Name        string
	Description string
	Handler     CommandHandler
}

// CommandInfo — сокращённая информация о зарегистрированной команде.
type CommandInfo struct {
	Name        string
	Description string
}

// Service инкапсулирует Max Bot API и распределяет входящие апдейты по обработчикам.
type Service struct {
	api *maxbot.Api
	log zerolog.Logger

	updates UpdatesProvider
	sender  MessageSender

	mu               sync.RWMutex
	commandOrder     []string
	commands         map[string]commandEntry
	messageHandlers  []MessageHandler
	callbackHandlers []CallbackHandler

	sessions        map[int64]SessionState
	sessionHandlers map[string]SessionHandler
}

type commandEntry struct {
	description string
	handler     CommandHandler
}

// SessionState описывает текущее состояние пользователя при пошаговом вводе.
type SessionState struct {
	Step   string
	Params map[string]string
}

// SessionHandler вызывается, когда у пользователя есть активный шаг сессии.
type SessionHandler func(context.Context, *MessageContext, SessionState) error

// NewService связывает клиент Max API и логгер.
func NewService(api *maxbot.Api, log zerolog.Logger) *Service {
	if api == nil {
		panic("appbot: api client is nil")
	}

	if api.Messages == nil {
		panic("appbot: message sender is nil")
	}

	return &Service{
		api:             api,
		log:             log.With().Str("component", "appbot").Logger(),
		updates:         api,
		sender:          api.Messages,
		commands:        make(map[string]commandEntry),
		sessions:        make(map[int64]SessionState),
		sessionHandlers: make(map[string]SessionHandler),
	}
}

// RegisterCommand потокобезопасно добавляет или переопределяет обработчик команды.
func (s *Service) RegisterCommand(cmd Command) {
	name := normalizeCommand(cmd.Name)
	if name == "" || cmd.Handler == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.commands[name]; !exists {
		s.commandOrder = append(s.commandOrder, name)
	}

	s.commands[name] = commandEntry{
		description: cmd.Description,
		handler:     cmd.Handler,
	}
}

// RegisterMessageHandler добавляет запасной обработчик, если ни одна команда не подошла.
func (s *Service) RegisterMessageHandler(handler MessageHandler) {
	if handler == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageHandlers = append(s.messageHandlers, handler)
}

// RegisterCallbackHandler добавляет обработчик для нажатий на кнопки.
func (s *Service) RegisterCallbackHandler(handler CallbackHandler) {
	if handler == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbackHandlers = append(s.callbackHandlers, handler)
}

// RegisterSessionHandler регистрирует обработчик для указанного шага сессии.
func (s *Service) RegisterSessionHandler(step string, handler SessionHandler) {
	if step == "" || handler == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessionHandlers == nil {
		s.sessionHandlers = make(map[string]SessionHandler)
	}
	s.sessionHandlers[step] = handler
}

// Commands возвращает команды в порядке регистрации.
func (s *Service) Commands() []CommandInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]CommandInfo, 0, len(s.commandOrder))
	for _, name := range s.commandOrder {
		entry := s.commands[name]
		result = append(result, CommandInfo{
			Name:        name,
			Description: entry.description,
		})
	}
	return result
}

// API возвращает клиент Max Bot API.
func (s *Service) API() *maxbot.Api {
	return s.api
}

// SendMessage отправляет подготовленное сообщение через API бота.
func (s *Service) SendMessage(ctx context.Context, msg *maxbot.Message) (string, error) {
	if msg == nil {
		return "", errors.New("appbot: message is nil")
	}
	if s.sender == nil {
		return "", fmt.Errorf("appbot: message sender is nil")
	}
	return s.sender.Send(ctx, msg)
}

// NewKeyboardBuilder возвращает хелпер для сборки инлайн-клавиатур.
func (s *Service) NewKeyboardBuilder() *maxbot.Keyboard {
	if s.api == nil || s.api.Messages == nil {
		return nil
	}
	return s.api.Messages.NewKeyboardBuilder()
}

// SetSessionState сохраняет состояние пользователя.
func (s *Service) SetSessionState(userID int64, state SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setSessionStateLocked(userID, state)
}

func (s *Service) setSessionStateLocked(userID int64, state SessionState) {
	if userID == 0 || state.Step == "" {
		delete(s.sessions, userID)
		return
	}
	if s.sessions == nil {
		s.sessions = make(map[int64]SessionState)
	}
	s.sessions[userID] = SessionState{
		Step:   state.Step,
		Params: cloneParams(state.Params),
	}
}

// ClearSessionState удаляет состояние пользователя.
func (s *Service) ClearSessionState(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, userID)
}

// SessionState возвращает копию сохранённого состояния пользователя.
func (s *Service) SessionState(userID int64) (SessionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.sessions[userID]
	if !ok {
		return SessionState{}, false
	}
	return SessionState{
		Step:   state.Step,
		Params: cloneParams(state.Params),
	}, true
}

// Run читает апдейты из API до отмены контекста.
func (s *Service) Run(ctx context.Context) error {
	if s.updates == nil {
		return errors.New("appbot: updates provider is nil")
	}

	updates := s.updates.GetUpdates(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				if err := ctx.Err(); err != nil {
					return err
				}
				return nil
			}
			if err := s.processUpdate(ctx, update); err != nil && err.Error() != "" && !errors.Is(err, context.Canceled) {
				s.log.Error().
					Err(err).
					Str("update_type", string(update.GetUpdateType())).
					Msg("failed to process update")
			}
		}
	}
}

func (s *Service) processUpdate(ctx context.Context, update schemes.UpdateInterface) error {
	s.logIncomingUpdate(update)

	switch upd := update.(type) {
	case *schemes.MessageCreatedUpdate:
		return s.handleMessage(ctx, upd)
	case *schemes.MessageCallbackUpdate:
		return s.handleCallback(ctx, upd)

	default:
		s.log.Debug().
			Str("update_type", string(update.GetUpdateType())).
			Msg("update ignored (no handler)")
		return nil
	}
}

func (s *Service) logIncomingUpdate(update schemes.UpdateInterface) {
	if update == nil {
		return
	}

	event := s.log.Info().
		Str("update_type", string(update.GetUpdateType()))

	switch upd := update.(type) {
	case *schemes.MessageCreatedUpdate:
		text := strings.TrimSpace(upd.GetText())
		event = event.
			Int64("user_id", upd.GetUserID()).
			Int64("chat_id", upd.GetChatID()).
			Str("user_name", upd.Message.Sender.Name)
		if text != "" {
			event = event.Str("text", text)
		}
		event.Msg("incoming message request")
	case *schemes.MessageCallbackUpdate:
		event = event.
			Int64("user_id", upd.Callback.GetUserID()).
			Str("user_name", upd.Callback.User.Name).
			Str("payload", upd.Callback.Payload)
		if upd.Message != nil {
			event = event.Int64("chat_id", upd.Message.Recipient.ChatId)
		}
		event.Msg("incoming callback request")
	default:
		event.Msg("incoming update request")
	}
}

func (s *Service) handleCallback(ctx context.Context, update *schemes.MessageCallbackUpdate) error {
	cbCtx := newCallbackContext(s, update)

	s.mu.RLock()
	handlers := append([]CallbackHandler{}, s.callbackHandlers...)
	s.mu.RUnlock()

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, cbCtx); err != nil && err.Error() != "" {
			return err
		}
	}

	return nil
}

func (s *Service) handleMessage(ctx context.Context, update *schemes.MessageCreatedUpdate) error {
	msgCtx := newMessageContext(s, update)

	var (
		command        commandEntry
		found          bool
		session        SessionState
		sessionHandler SessionHandler
		hasSession     bool
	)

	s.mu.RLock()
	if s.sessions != nil {
		if state, ok := s.sessions[msgCtx.SenderID()]; ok {
			session = SessionState{
				Step:   state.Step,
				Params: cloneParams(state.Params),
			}
			sessionHandler, hasSession = s.sessionHandlers[session.Step]
		}
	}
	if s.commands != nil {
		command, found = s.commands[msgCtx.Command()]
	}
	handlers := append([]MessageHandler{}, s.messageHandlers...)
	s.mu.RUnlock()

	if hasSession && sessionHandler != nil {
		if err := sessionHandler(ctx, msgCtx, session); err != nil && err.Error() != "" {
			return fmt.Errorf("session %q: %w", session.Step, err)
		}
		return nil
	}

	if found && command.handler != nil {
		if err := command.handler(ctx, msgCtx); err != nil && err.Error() != "" {
			return fmt.Errorf("command %q: %w", msgCtx.Command(), err)
		}
		return nil
	}

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, msgCtx); err != nil && err.Error() != "" {
			return err
		}
	}

	return nil
}

func normalizeCommand(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	return strings.ToLower(name)
}

func (s *Service) sendText(ctx context.Context, text string, userID, chatID int64) error {
	if text == "" {
		return nil
	}

	msg := maxbot.NewMessage().SetText(text)
	if userID != 0 {
		msg.SetUser(userID)
	}
	if chatID != 0 {
		msg.SetChat(chatID)
	}

	if s.sender == nil {
		return fmt.Errorf("appbot: message sender is nil")
	}

	_, err := s.sender.Send(ctx, msg)
	return err
}

func cloneParams(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
