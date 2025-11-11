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
	title, keyboard, err := mr.renderMenu(menuID)
	if err != nil {
		return err
	}

	msg := maxbot.NewMessage().SetText(title)
	if userID != 0 {
		msg.SetUser(userID)
	}
	if chatID != 0 {
		msg.SetChat(chatID)
	}
	msg.AddKeyboard(keyboard)

	_, err = mr.bot.SendMessage(ctx, msg)
	return err
}

func (mr *MenuRegistry) buildMenuBody(menuID string) (*schemes.NewMessageBody, error) {
	title, keyboard, err := mr.renderMenu(menuID)
	if err != nil {
		return nil, err
	}
	body := &schemes.NewMessageBody{
		Text: title,
	}
	body.Attachments = append(body.Attachments, schemes.NewInlineKeyboardAttachmentRequest(keyboard.Build()))
	return body, nil
}

func (mr *MenuRegistry) renderMenu(menuID string) (string, *maxbot.Keyboard, error) {
	menu, ok := mr.menus[menuID]
	if !ok {
		return "", nil, fmt.Errorf("menu %q is not registered", menuID)
	}

	builder := mr.bot.NewKeyboardBuilder()
	if builder == nil {
		return "", nil, fmt.Errorf("menu: keyboard builder is nil")
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

	return menu.Title, builder, nil
}
