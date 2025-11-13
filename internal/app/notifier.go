package app

import (
	"context"
	"fmt"

	"github.com/c4erries/max_bot/internal/appbot"
)

// NotificationService adapts bot service for HTTP notifications.
type NotificationService struct {
	bot *appbot.Service
}

// NewNotifier wires the bot service so HTTP handlers can reuse complex flows.
func NewNotifier(bot *appbot.Service) *NotificationService {
	if bot == nil {
		panic("app: notifier bot service is nil")
	}
	return &NotificationService{bot: bot}
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
