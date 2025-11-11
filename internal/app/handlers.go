package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c4erries/max_bot/internal/appbot"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

const (
	menuRoot                = "menu:root"
	menuSchedule            = "menu:schedule"
	menuApplicationsStudent = "menu:applications:student"
	menuApplicationsTeacher = "menu:applications:teacher"

	actionPaymentRequestOrder             = "action:payment:request_order"
	actionPaymentDormPay                  = "action:payment:pay_dorm"
	actionPaymentTuitionPay               = "action:payment:pay_tuition"
	actionScheduleToday                   = "action:schedule:today"
	actionApplicationsOpen                = "action:applications:open"
	actionApplicationStudentStudyCert     = "action:application:student:study_certificate"
	actionApplicationStudentAcademicLeave = "action:application:student:academic_leave"
	actionApplicationStudentTransfer      = "action:application:student:study_transfer"
	actionApplicationTeacherWorkCert      = "action:application:teacher:work_certificate"

	sessionApplicationFilling = "application:filling"
)

type applicationActionMeta struct {
	role applicationRole
	doc  applicationType
}

var applicationActionPayloads = map[string]applicationActionMeta{
	actionApplicationStudentStudyCert: {
		role: roleStudent,
		doc:  applicationTypeStudyCertificate,
	},
	actionApplicationStudentAcademicLeave: {
		role: roleStudent,
		doc:  applicationTypeAcademicLeave,
	},
	actionApplicationStudentTransfer: {
		role: roleStudent,
		doc:  applicationTypeStudyTransfer,
	},
	actionApplicationTeacherWorkCert: {
		role: roleTeacher,
		doc:  applicationTypeWorkCertificate,
	},
}

func registerDefaultBotHandlers(bot *appbot.Service, applications *applicationCoordinator, payments *paymentService, schedule *scheduleService) {
	if bot == nil {
		panic("app: bot service is nil")
	}
	if applications == nil {
		panic("app: application coordinator is nil")
	}
	if payments == nil {
		panic("app: payment service is nil")
	}
	if schedule == nil {
		panic("app: schedule service is nil")
	}
	menus := NewMenuRegistry(bot)
	registerMenus(menus)

	bot.RegisterBotStartedHandler(func(ctx context.Context, start *appbot.BotStartedContext) error {
		if err := menus.Send(ctx, start.ChatID(), start.UserID(), menuRoot); err != nil && err.Error() != "" {
			logger := start.Logger()
			logger.Error().Err(err).Msg("failed to send menu on bot start")
			return start.ReplyText(ctx, "Главное меню временно недоступно. Отправьте /start чуть позже.")
		}
		return nil
	})

	bot.RegisterCommand(appbot.Command{
		Name:        "start",
		Description: "Показать главное меню",
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
		Description: "Показать список доступных команд",
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
		metaAction, hasApplicationAction := applicationActionPayloads[payload]
		switch {
		case strings.HasPrefix(payload, "menu:"):
			if err := menus.Send(ctx, cb.ChatID(), cb.SenderID(), payload); err != nil && err.Error() != "" {
				logger := cb.Logger()
				logger.Error().Err(err).Str("menu_id", payload).Msg("failed to send menu")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Меню временно недоступно"})
			}
			return cb.Answer(ctx, nil)

		case payload == actionApplicationsOpen:
			role, err := applications.ResolveRole(ctx, cb.SenderID())
			if err != nil {
				logger := cb.Logger()
				logger.Error().Err(err).Msg("failed to resolve user role")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось определить доступные заявки"})
			}
			var menuID string
			switch role {
			case roleStudent:
				menuID = menuApplicationsStudent
			case roleTeacher:
				menuID = menuApplicationsTeacher
			}
			if menuID == "" {
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось определить доступные заявки"})
			}
			if err := menus.Send(ctx, cb.ChatID(), cb.SenderID(), menuID); err != nil && err.Error() != "" {
				logger := cb.Logger()
				logger.Error().Err(err).Str("menu_id", menuID).Msg("failed to send applications menu")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось открыть список заявок"})
			}
			return cb.Answer(ctx, nil)
		case hasApplicationAction:
			sessionData, err := applications.PrepareSession(cb.SenderID(), metaAction.role, metaAction.doc)
			if err != nil {
				logger := cb.Logger()
				logger.Error().Err(err).Str("doc_type", string(metaAction.doc)).Msg("failed to load application form")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось загрузить форму. Попробуйте позже"})
			}
			if sessionData.StepsCount() == 0 {
				if err := applications.Submit(ctx, cb.SenderID(), sessionData); err != nil {
					logger := cb.Logger()
					logger.Error().Err(err).Str("doc_type", string(metaAction.doc)).Msg("failed to submit auto form")
					return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось отправить заявку. Попробуйте позже"})
				}
				if err := cb.Answer(ctx, nil); err != nil {
					return err
				}
				if err := cb.ReplyText(ctx, formatSuccessMessage(sessionData.FormTitle)); err != nil {
					return err
				}
				return menus.Send(ctx, cb.ChatID(), cb.SenderID(), menuRoot)
			}
			payloadBytes, err := sessionData.marshal()
			if err != nil {
				logger := cb.Logger()
				logger.Error().Err(err).Str("doc_type", string(metaAction.doc)).Msg("failed to encode application session")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось подготовить форму. Попробуйте позже"})
			}
			cb.SetSessionState(appbot.SessionState{
				Step: sessionApplicationFilling,
				Params: map[string]string{
					"form_type": string(metaAction.doc),
					"role":      string(metaAction.role),
				},
				Payload: payloadBytes,
			})
			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}
			return cb.ReplyText(ctx, sessionData.StartPrompt())
		case payload == actionPaymentRequestOrder:
			status, err := payments.Status(ctx, cb.SenderID())
			if err != nil {
				logger := cb.Logger()
				logger.Error().Err(err).Msg("failed to fetch payment status")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось проверить оплату. Попробуйте позже"})
			}
			if !status.NeedDorm && !status.NeedTuition {
				if err := cb.Answer(ctx, nil); err != nil {
					return err
				}
				return cb.ReplyText(ctx, "Оплата не требуется — задолженностей нет.")
			}

			builder := cb.Service().NewKeyboardBuilder()
			if builder == nil {
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось построить меню оплат"})
			}

			row := builder.AddRow()
			if status.NeedDorm {
				row.AddCallback("Оплатить общежитие", schemes.POSITIVE, actionPaymentDormPay)
			}
			if status.NeedTuition {
				if status.NeedDorm {
					row = builder.AddRow()
				}
				row.AddCallback("Оплатить обучение", schemes.POSITIVE, actionPaymentTuitionPay)
			}

			backRow := builder.AddRow()
			backRow.AddCallback("Назад", schemes.DEFAULT, menuRoot)

			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}

			msg := maxbot.NewMessage().SetText("Выберите платеж, который хотите внести:")
			msg.SetUser(cb.SenderID())
			msg.SetChat(cb.ChatID())
			msg.AddKeyboard(builder)
			_, sendErr := cb.Service().SendMessage(ctx, msg)
			return sendErr
		case payload == actionPaymentDormPay:
			return sendPaymentLink(ctx, cb, payments, paymentKindDorm, "Оплатить общежитие можно по ссылке: %s")
		case payload == actionPaymentTuitionPay:
			return sendPaymentLink(ctx, cb, payments, paymentKindTuition, "Оплатить обучение можно по ссылке: %s")
		case payload == actionScheduleToday:
			text, err := schedule.Today(ctx, cb.SenderID())
			if err != nil {
				logger := cb.Logger()
				logger.Error().Err(err).Msg("failed to fetch schedule")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось получить расписание"})
			}
			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}
			return cb.ReplyText(ctx, text)
		default:
			return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Неизвестное действие"})
		}
	})

	bot.RegisterSessionHandler(sessionApplicationFilling, func(ctx context.Context, msg *appbot.MessageContext, state appbot.SessionState) error {
		progress, err := applicationSessionFromPayload(state.Payload)
		if err != nil {
			logger := msg.Logger()
			logger.Error().Err(err).Msg("failed to restore application session")
			msg.ClearSessionState()
			return msg.ReplyText(ctx, "Не удалось восстановить заявку. Пожалуйста, начните оформление заново через меню.")
		}

		field, ok := progress.currentField()
		if !ok {
			msg.ClearSessionState()
			return msg.ReplyText(ctx, "Заявка уже заполнена. Откройте меню и создайте новую, если нужно.")
		}

		switch field.Kind {
		case fieldKindFile:
			raw := msg.Update().Message.Body.RawAttachments
			if len(raw) == 0 {
				return msg.ReplyText(ctx, "Пожалуйста, прикрепите нужный файл к сообщению и отправьте его ещё раз.")
			}
			payload, err := encodeAttachments(raw)
			if err != nil {
				logger := msg.Logger()
				logger.Error().Err(err).Msg("failed to encode attachments")
				return msg.ReplyText(ctx, "Не удалось обработать файл. Отправьте его ещё раз.")
			}
			progress.RecordFileAnswer(payload)
		default:
			answer := strings.TrimSpace(msg.Text())
			if field.Required && answer == "" {
				return msg.ReplyText(ctx, progress.ReminderForRequiredField())
			}
			progress.RecordAnswer(answer)
		}

		if progress.IsCompleted() {
			if err := applications.Submit(ctx, msg.SenderID(), progress); err != nil {
				logger := msg.Logger()
				logger.Error().Err(err).Msg("failed to submit application")
				msg.ClearSessionState()
				return msg.ReplyText(ctx, "Не удалось отправить заявку. Попробуйте повторить чуть позже.")
			}

			msg.ClearSessionState()
			if err := msg.ReplyText(ctx, formatSuccessMessage(progress.FormTitle)); err != nil {
				return err
			}
			if err := menus.Send(ctx, msg.ChatID(), msg.SenderID(), menuRoot); err != nil && err.Error() != "" {
				logger := msg.Logger()
				logger.Error().Err(err).Msg("failed to send menu after application submission")
				return msg.ReplyText(ctx, "Главное меню сейчас недоступно. Вызовите /start позднее.")
			}
			return nil
		}

		payloadBytes, err := progress.marshal()
		if err != nil {
			logger := msg.Logger()
			logger.Error().Err(err).Msg("failed to encode application session")
			msg.ClearSessionState()
			return msg.ReplyText(ctx, "Не удалось сохранить ответ. Перезапустите оформление заявки.")
		}

		newParams := map[string]string{}
		for k, v := range state.Params {
			newParams[k] = v
		}

		msg.SetSessionState(appbot.SessionState{
			Step:    sessionApplicationFilling,
			Params:  newParams,
			Payload: payloadBytes,
		})

		return msg.ReplyText(ctx, progress.NextPrompt())
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
		Title: "Главное меню: выберите раздел",
		Rows: [][]MenuButton{
			{
				{Text: "Платежи", Payload: actionPaymentRequestOrder, Intent: schemes.POSITIVE},
				{Text: "Расписание", Payload: menuSchedule, Intent: schemes.DEFAULT},
			},
			{
				{Text: "Заявления", Payload: actionApplicationsOpen, Intent: schemes.POSITIVE},
			},
		},
	})

	menus.Register(Menu{
		ID:    menuSchedule,
		Title: "Расписание:",
		Rows: [][]MenuButton{
			{
				{Text: "Показать расписание на сегодня", Payload: actionScheduleToday, Intent: schemes.DEFAULT},
			},
			{
				{Text: "Назад", Payload: menuRoot, Intent: schemes.DEFAULT},
			},
		},
	})

	menus.Register(Menu{
		ID:    menuApplicationsStudent,
		Title: "Заявления студентов:",
		Rows: [][]MenuButton{
			{
				{Text: "Справка с места учебы", Payload: actionApplicationStudentStudyCert, Intent: schemes.POSITIVE},
			},
			{
				{Text: "Академический отпуск", Payload: actionApplicationStudentAcademicLeave, Intent: schemes.POSITIVE},
			},
			{
				{Text: "Перевод на другую программу", Payload: actionApplicationStudentTransfer, Intent: schemes.POSITIVE},
			},
			{
				{Text: "Назад", Payload: menuRoot, Intent: schemes.DEFAULT},
			},
		},
	})

	menus.Register(Menu{
		ID:    menuApplicationsTeacher,
		Title: "Заявления преподавателей:",
		Rows: [][]MenuButton{
			{
				{Text: "Справка с места работы", Payload: actionApplicationTeacherWorkCert, Intent: schemes.POSITIVE},
			},
			{
				{Text: "Назад", Payload: menuRoot, Intent: schemes.DEFAULT},
			},
		},
	})
}

// encodeAttachments приводит список вложений к JSON-строке,
// чтобы backend смог восстановить исходные файлы.
func encodeAttachments(raw []json.RawMessage) (string, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// sendPaymentLink запрашивает ссылку на оплату и отправляет её пользователю.
func sendPaymentLink(ctx context.Context, cb *appbot.CallbackContext, payments *paymentService, kind paymentKind, template string) error {
	link, err := payments.Link(ctx, cb.SenderID(), kind)
	if err != nil {
		logger := cb.Logger()
		logger.Error().Err(err).Str("payment_kind", string(kind)).Msg("failed to create payment link")
		return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось сформировать ссылку на оплату"})
	}
	if err := cb.Answer(ctx, nil); err != nil {
		return err
	}
	return cb.ReplyText(ctx, fmt.Sprintf(template, link))
}
