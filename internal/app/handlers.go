package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/c4erries/max_bot/internal/appbot"
	"github.com/c4erries/max_bot/internal/backend"
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
	actionScheduleWeek                    = "action:schedule:week"
	actionApplicationsOpen                = "action:applications:open"
	actionApplicationStudentStudyCert     = "action:application:student:study_certificate"
	actionApplicationStudentAcademicLeave = "action:application:student:academic_leave"
	actionApplicationStudentTransfer      = "action:application:student:study_transfer"
	actionApplicationTeacherWorkCert      = "action:application:teacher:work_certificate"
	actionApplicationCancel               = "action:application:cancel"
	actionReadyDocumentPickup             = "action:ready_document:pickup"
	actionReadyDocumentEmail              = "action:ready_document:email"

	sessionApplicationFilling = "application:filling"
	sessionReadyDocumentEmail = "ready_document:email"
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

const (
	startGreetingText = `Привет! 👋
Я — твой цифровой помощник в университете. 🎓✨

Я помогу:
→ 🧑‍🎓 Студентам: смотреть расписание, подавать заявки на справки и отпуска, отслеживать статусы и многое другое.
→ 👨‍🏫 Преподавателям и сотрудникам: управлять расписанием, согласовывать заявки и упростить документооборот.

Чтобы начать, нам нужно тебя узнать.
Все данные нужны, чтобы показывать только твоё расписание и давать доступ к личным документам. 🔒

Давай начнём? 🚀`
	readyDocumentNotificationText = `🎉 Ваша заявка готова!

✅ Статус: Обработана и готова к получению

Теперь вы можете:
• Забрать оригинал в деканате 📍
• Запросить отправку на вашу электронную почту 📧

Выберите удобный способ получения!`
	readyDocumentPickupText = `✅ Отлично! Ваша справка уже ждёт вас в деканате. 📄

📍 Не забудьте взять с собой студенческий билет или паспорт.

Часы работы деканата:
Пн-Пт: с 9:00 до 18:00
Обед: с 13:00 до 14:00

Желаем хорошего дня! 😊`
	readyDocumentEmailPromptText = `Хорошо! Чтобы отправить справку на email, пришлите нам, пожалуйста, вашу рабочую почту.
📧 Убедитесь, что почта корректна, чтобы письмо не потерялось.`
	readyDocumentEmailInvalidText = `Пожалуйста, укажите корректный рабочий email. Например: ivan.ivanov@university.ru`
	readyDocumentEmailSuccessText = `Отлично! Ваша справка с места работы была направлена на указанную электронную почту. 📨

Что делать дальше:

Проверьте входящие сообщения, а также папку «Спам», если письмо не пришло в течение 15 минут.

Если вы не получили письмо, пожалуйста, сообщите нам об этом.`
)

var emailRegexp = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

func registerDefaultBotHandlers(bot *appbot.Service, applications *applicationCoordinator, payments backend.Payments, schedule *scheduleService) {
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
		if err := start.ReplyText(ctx, startGreetingText); err != nil && err.Error() != "" {
			logger := start.Logger()
			logger.Warn().Err(err).Msg("failed to send greeting on bot start")
		}
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
			if err := sendMenuFromCallback(ctx, menus, cb, payload); err != nil && err.Error() != "" {
				logger := cb.Logger()
				logger.Error().Err(err).Str("menu_id", payload).Msg("failed to send menu")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Меню временно недоступно"})
			}
			return nil

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
			if err := sendMenuFromCallback(ctx, menus, cb, menuID); err != nil && err.Error() != "" {
				logger := cb.Logger()
				logger.Error().Err(err).Str("menu_id", menuID).Msg("failed to send applications menu")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось открыть список заявок"})
			}
			return nil
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
			return sendApplicationPrompt(ctx, cb.Service(), cb.ChatID(), cb.SenderID(), sessionData.StartPrompt())
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
				row.AddCallback("💳 Оплатить общежитие", schemes.POSITIVE, actionPaymentDormPay)
			}
			if status.NeedTuition {
				if status.NeedDorm {
					row = builder.AddRow()
				}
				row.AddCallback("💳 Оплатить обучение", schemes.POSITIVE, actionPaymentTuitionPay)
			}

			backRow := builder.AddRow()
			backRow.AddCallback("Назад", schemes.DEFAULT, menuRoot)

			body := &schemes.NewMessageBody{
				Text: `Оплата услуг 🔒

Ваша безопасность — наш приоритет. Все платежи защищены.`,
			}
			body.Attachments = append(body.Attachments, schemes.NewInlineKeyboardAttachmentRequest(builder.Build()))

			if err := cb.Answer(ctx, &schemes.CallbackAnswer{Message: body}); err == nil {
				return nil
			} else {
				logger := cb.Logger()
				logger.Warn().Err(err).Msg("failed to update payments menu via callback answer, fallback to sending new one")
			}

			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}

			msg := maxbot.NewMessage().SetText(body.Text)
			msg.SetUser(cb.SenderID())
			msg.SetChat(cb.ChatID())
			msg.AddKeyboard(builder)
			_, sendErr := cb.Service().SendMessage(ctx, msg)
			return sendErr
		case payload == actionPaymentDormPay:
			return sendPaymentLink(ctx, cb, payments, backend.PaymentKindDorm, "Оплатить общежитие можно по ссылке: %s")
		case payload == actionPaymentTuitionPay:
			return sendPaymentLink(ctx, cb, payments, backend.PaymentKindTuition, "Оплатить обучение можно по ссылке: %s")
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
		case payload == actionScheduleWeek:
			text, err := schedule.Week(ctx, cb.SenderID())
			if err != nil {
				logger := cb.Logger()
				logger.Error().Err(err).Msg("failed to fetch weekly schedule")
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Не удалось получить недельное расписание"})
			}
			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}
			return cb.ReplyText(ctx, text)
		case payload == actionReadyDocumentPickup:
			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}
			return cb.ReplyText(ctx, readyDocumentPickupText)
		case payload == actionReadyDocumentEmail:
			cb.SetSessionState(appbot.SessionState{
				Step: sessionReadyDocumentEmail,
			})
			if err := cb.Answer(ctx, nil); err != nil {
				return err
			}
			return cb.ReplyText(ctx, readyDocumentEmailPromptText)
		case payload == actionApplicationCancel:
			state, ok := cb.SessionState()
			if !ok || state.Step != sessionApplicationFilling {
				return cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Нет заявки для отмены"})
			}
			cb.ClearSessionState()
			if err := cb.Answer(ctx, &schemes.CallbackAnswer{Notification: "Заполнение отменено"}); err != nil {
				return err
			}
			if err := cb.ReplyText(ctx, "Заполнение заявки остановлено. Можете начать заново через меню."); err != nil {
				return err
			}
			return menus.Send(ctx, cb.ChatID(), cb.SenderID(), menuRoot)
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

		return sendApplicationPrompt(ctx, msg.Service(), msg.ChatID(), msg.SenderID(), progress.NextPrompt())
	})

	bot.RegisterSessionHandler(sessionReadyDocumentEmail, func(ctx context.Context, msg *appbot.MessageContext, state appbot.SessionState) error {
		email := strings.TrimSpace(msg.Text())
		if email == "" {
			return msg.ReplyText(ctx, readyDocumentEmailPromptText)
		}
		if !emailRegexp.MatchString(email) {
			return msg.ReplyText(ctx, readyDocumentEmailInvalidText)
		}

		msg.ClearSessionState()
		if err := msg.ReplyText(ctx, readyDocumentEmailSuccessText); err != nil {
			return err
		}
		return nil
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
		ID: menuRoot,
		Title: `Добро пожаловать в главное меню! 🎓

Выберите, что вас интересует:

1. Платежи 💳 — Проверить баланс и оплатить обучение или общежитие.
2. Расписание 📚 — Посмотреть ваше расписание на текущую неделю.
3. Заявления 📄 — Подать заявку на справку, академический отпуск или перевод.

Просто нажмите на одну из кнопок ниже, чтобы продолжить! 👇`,
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
		ID: menuSchedule,
		Title: `📅 Какое расписание вас интересует?

Выберите вариант ниже, чтобы посмотреть`,
		Rows: [][]MenuButton{
			{
				{Text: "Сегодня", Payload: actionScheduleToday, Intent: schemes.DEFAULT},
				{Text: "На эту неделю", Payload: actionScheduleWeek, Intent: schemes.DEFAULT},
			},
			{
				{Text: "Назад", Payload: menuRoot, Intent: schemes.DEFAULT},
			},
		},
	})

	menus.Register(Menu{
		ID:    menuApplicationsStudent,
		Title: "📄 Выберите тип заявления, которое хотите подать:",
		Rows: [][]MenuButton{
			{
				{Text: "Справка с места обучения 🎓", Payload: actionApplicationStudentStudyCert, Intent: schemes.POSITIVE},
			},
			{
				{Text: "Справка об уходе в академ 📅", Payload: actionApplicationStudentAcademicLeave, Intent: schemes.POSITIVE},
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

func sendMenuFromCallback(ctx context.Context, menus *MenuRegistry, cb *appbot.CallbackContext, menuID string) error {
	body, err := menus.buildMenuBody(menuID)
	if err != nil {
		return err
	}
	answer := &schemes.CallbackAnswer{Message: body}
	if err := cb.Answer(ctx, answer); err != nil {
		logger := cb.Logger()
		logger.Warn().
			Err(err).
			Str("menu_id", menuID).
			Msg("failed to update menu via callback answer, fallback to sending new one")
		return menus.Send(ctx, cb.ChatID(), cb.SenderID(), menuID)
	}
	return nil
}

func sendApplicationPrompt(ctx context.Context, svc *appbot.Service, chatID, userID int64, text string) error {
	if svc == nil {
		return fmt.Errorf("application prompt sender is nil")
	}

	msg := maxbot.NewMessage().SetText(text)
	if userID != 0 {
		msg.SetUser(userID)
	}
	if chatID != 0 {
		msg.SetChat(chatID)
	}

	if builder := svc.NewKeyboardBuilder(); builder != nil {
		builder.AddRow().AddCallback("Отменить заполнение", schemes.NEGATIVE, actionApplicationCancel)
		msg.AddKeyboard(builder)
	}

	_, err := svc.SendMessage(ctx, msg)
	return err
}

func sendReadyNotification(ctx context.Context, svc *appbot.Service, userID int64) error {
	if svc == nil {
		return fmt.Errorf("ready notification sender is nil")
	}
	if userID <= 0 {
		return fmt.Errorf("ready notification user id must be positive")
	}

	msg := maxbot.NewMessage().SetText(readyDocumentNotificationText)
	msg.SetUser(userID)

	builder := svc.NewKeyboardBuilder()
	if builder == nil {
		return fmt.Errorf("ready notification keyboard builder is nil")
	}
	row := builder.AddRow()
	row.AddCallback("Забрать в деканате", schemes.POSITIVE, actionReadyDocumentPickup)
	row.AddCallback("Отправить на почту", schemes.DEFAULT, actionReadyDocumentEmail)
	msg.AddKeyboard(builder)

	_, err := svc.SendMessage(ctx, msg)
	return err
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
func sendPaymentLink(ctx context.Context, cb *appbot.CallbackContext, payments backend.Payments, kind backend.PaymentKind, template string) error {
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