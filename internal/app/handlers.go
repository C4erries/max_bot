package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/c4erries/max_bot/internal/appbot"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

const (
	menuRoot     = "menu:root"
	menuPayments = "menu:payments"
	menuSchedule = "menu:schedule"

	actionPaymentRequestOrder = "action:payment:request_order"
	actionScheduleToday       = "action:schedule:today"

	sessionPaymentWaitingOrder = "payment:waiting_order"
)

func registerDefaultBotHandlers(bot *appbot.Service) {
	menus := NewMenuRegistry(bot)
	registerMenus(menus)

	bot.RegisterCommand(appbot.Command{
		Name:        "start",
		Description: "Show the main menu",
		Handler: func(ctx context.Context, msg *appbot.MessageContext) error {
			msg.ClearSessionState()
			if err := menus.Send(ctx, msg.ChatID(), msg.SenderID(), menuRoot); err != nil && err.Error() != "" {
				logger := msg.Logger()
				logger.Error().Err(err).Msg("failed to send root menu")
				return msg.ReplyText(ctx, "Привет! Пока не могу показать меню, попробуйте позже.")
			}
			return nil
		},
	})

	bot.RegisterCommand(appbot.Command{
		Name:        "help",
		Description: "Display the list of available commands",
		Handler: func(ctx context.Context, msg *appbot.MessageContext) error {
			commands := bot.Commands()
			if len(commands) == 0 {
				return msg.ReplyText(ctx, "Команды ещё не подключены. Попробуйте позже.")
			}

			var b strings.Builder
			b.WriteString("Доступные команды:\n")
			for _, cmd := range commands {
				desc := cmd.Description
				if desc == "" {
					desc = "описание появится позже"
				}
				b.WriteString(fmt.Sprintf("/%s - %s\n", cmd.Name, desc))
			}
			b.WriteString("\nЧтобы открыть меню, введите /start или нажмите кнопку ниже.")

			return msg.ReplyText(ctx, b.String())
		},
	})

	bot.RegisterCallbackHandler(func(ctx context.Context, cb *appbot.CallbackContext) error {
		payload := cb.Payload()
		switch {
		case strings.HasPrefix(payload, "menu:"):
			if err := menus.Send(ctx, cb.ChatID(), cb.SenderID(), payload); err != nil && err.Error() != "" {
				logger := cb.Logger()
				logger.Error().Err(err).Str("menu_id", payload).Msg("failed to send menu")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Меню временно недоступно"})
			}
			return cb.Answer(ctx, nil)
		case payload == actionPaymentRequestOrder:
			cb.SetSessionState(appbot.SessionState{Step: sessionPaymentWaitingOrder})
			if err := cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Жду номер заказа"}); err != nil {
				return err
			}
			return cb.ReplyText(ctx, "Пришлите номер заказа в следующем сообщении.")
		case payload == actionScheduleToday:
			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}
			return cb.ReplyText(ctx, "Сегодня все свободны. Проверьте меню позже для обновлений.")
		default:
			return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Неизвестное действие"})
		}
	})

	bot.RegisterSessionHandler(sessionPaymentWaitingOrder, func(ctx context.Context, msg *appbot.MessageContext, state appbot.SessionState) error {
		orderID := strings.TrimSpace(msg.Text())
		if orderID == "" {
			return msg.ReplyText(ctx, "Номер заказа не должен быть пустым. Напишите его ещё раз.")
		}

		msg.ClearSessionState()
		if err := msg.Replyf(ctx, "Заказ %s принят. Мы свяжемся с вами после проверки.", orderID); err != nil {
			return err
		}
		return menus.Send(ctx, msg.ChatID(), msg.SenderID(), menuRoot)
	})

	bot.RegisterMessageHandler(func(ctx context.Context, msg *appbot.MessageContext) error {
		text := strings.TrimSpace(msg.Text())
		if text == "" {
			return nil
		}
		switch strings.ToLower(text) {
		case "hi", "привет":
			return msg.ReplyText(ctx, "Привет! Жмите кнопки меню или команду /start.")
		case "меню":
			if err := menus.Send(ctx, msg.ChatID(), msg.SenderID(), menuRoot); err != nil && err.Error() != "" {
				logger := msg.Logger()
				logger.Error().Err(err).Msg("failed to send menu from text shortcut")
				return msg.ReplyText(ctx, "Не смог показать меню. Попробуйте /start чуть позже.")
			}
			return nil
		}

		if msg.Command() != "" {
			return msg.ReplyText(ctx, "Неизвестная команда. Введите /help, чтобы узнать, что уже работает.")
		}

		return msg.ReplyText(ctx, "Используйте меню или /help, чтобы посмотреть доступные действия.")
	})
}

func registerMenus(menus *MenuRegistry) {
	menus.Register(Menu{
		ID:    menuRoot,
		Title: "Главное меню. Выберите раздел:",
		Rows: [][]MenuButton{
			{
				{Text: "Оплата", Payload: menuPayments, Intent: schemes.POSITIVE},
				{Text: "Расписание", Payload: menuSchedule, Intent: schemes.DEFAULT},
			},
		},
	})

	menus.Register(Menu{
		ID:    menuPayments,
		Title: "Оплата: что нужно сделать?",
		Rows: [][]MenuButton{
			{
				{Text: "Ввести номер заказа", Payload: actionPaymentRequestOrder, Intent: schemes.POSITIVE},
			},
			{
				{Text: "Назад", Payload: menuRoot, Intent: schemes.DEFAULT},
			},
		},
	})

	menus.Register(Menu{
		ID:    menuSchedule,
		Title: "Расписание:",
		Rows: [][]MenuButton{
			{
				{Text: "На сегодня", Payload: actionScheduleToday, Intent: schemes.DEFAULT},
			},
			{
				{Text: "Назад", Payload: menuRoot, Intent: schemes.DEFAULT},
			},
		},
	})
}
