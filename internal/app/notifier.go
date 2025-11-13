package app

import (
	"context"
	"fmt"

	"github.com/c4erries/max_bot/internal/appbot"
	"github.com/c4erries/max_bot/internal/backend"
)

// NotificationService adapts bot service for HTTP notifications.
type NotificationService struct {
	bot      *appbot.Service
	payments backend.Payments
}

// NewNotifier wires the bot service so HTTP handlers can reuse complex flows.
func NewNotifier(bot *appbot.Service, payments backend.Payments) *NotificationService {
	if bot == nil {
		panic("app: notifier bot service is nil")
	}
	if payments == nil {
		panic("app: payments backend is nil")
	}
	return &NotificationService{bot: bot, payments: payments}
}

// NotifyUser sends plain text message to a user.
func (n *NotificationService) NotifyUser(ctx context.Context, userID int64, text string) error {
	if n == nil || n.bot == nil {
		return fmt.Errorf("app: notifier is not initialized")
	}
	return n.bot.NotifyUser(ctx, userID, text)
}

// NotifyDocumentReady delivers the ready-document inline menu flow.
func (n *NotificationService) NotifyDocumentReady(ctx context.Context, userID int64) error {
	if n == nil || n.bot == nil {
		return fmt.Errorf("app: notifier is not initialized")
	}
	return sendReadyNotification(ctx, n.bot, userID)
}

const tuitionReminderTemplate = `Привет! 👋

Это напоминание, что настало время оплатить обучение за новый месяц.
Перейдите по ссылке и завершите платёж: %s

Если вы уже оплатили — просто игнорируйте это сообщение.`

// NotifyTuitionPaymentReminder sends tuition payment reminder with a fresh payment link.
func (n *NotificationService) NotifyTuitionPaymentReminder(ctx context.Context, userID int64) error {
	if n == nil || n.bot == nil {
		return fmt.Errorf("app: notifier is not initialized")
	}
	if n.payments == nil {
		return fmt.Errorf("app: payments backend is not configured")
	}

	link, err := n.payments.Link(ctx, userID, backend.PaymentKindTuition)
	if err != nil {
		return fmt.Errorf("app: tuition reminder payment link: %w", err)
	}

	return n.bot.NotifyUser(ctx, userID, fmt.Sprintf(tuitionReminderTemplate, link))
}
