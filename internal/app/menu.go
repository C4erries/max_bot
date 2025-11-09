package app

import (
	"context"
	"fmt"

	"github.com/c4erries/max_bot/internal/appbot"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

// Menu описывает текст и клавиатуру, которые нужно показать пользователю.
type Menu struct {
	ID    string
	Title string
	Rows  [][]MenuButton
}

// MenuButton описывает кнопку меню.
type MenuButton struct {
	Text    string
	Payload string
	Intent  schemes.Intent
}

// MenuRegistry хранит набор меню и умеет их отправлять.
type MenuRegistry struct {
	bot   *appbot.Service
	menus map[string]Menu
}

// NewMenuRegistry создаёт новый реестр меню.
func NewMenuRegistry(bot *appbot.Service) *MenuRegistry {
	return &MenuRegistry{
		bot:   bot,
		menus: make(map[string]Menu),
	}
}

// Register добавляет/обновляет меню по идентификатору.
func (mr *MenuRegistry) Register(menu Menu) {
	if menu.ID == "" {
		return
	}
	mr.menus[menu.ID] = menu
}

// Send отправляет указанное меню пользователю.
func (mr *MenuRegistry) Send(ctx context.Context, chatID, userID int64, menuID string) error {
	menu, ok := mr.menus[menuID]
	if !ok {
		return fmt.Errorf("menu %q is not registered", menuID)
	}

	builder := mr.bot.NewKeyboardBuilder()
	if builder == nil {
		return fmt.Errorf("menu: keyboard builder is nil")
	}

	for _, row := range menu.Rows {
		if len(row) == 0 {
			continue
		}
		kbRow := builder.AddRow()
		for _, btn := range row {
			if btn.Text == "" || btn.Payload == "" {
				continue
			}
			kbRow.AddCallback(btn.Text, btn.Intent, btn.Payload)
		}
	}

	msg := maxbot.NewMessage().SetText(menu.Title)
	if userID != 0 {
		msg.SetUser(userID)
	}
	if chatID != 0 {
		msg.SetChat(chatID)
	}
	msg.AddKeyboard(builder)

	_, err := mr.bot.SendMessage(ctx, msg)
	return err
}
