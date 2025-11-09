package appbot

import (
	"context"
	"io"
	"testing"

	"github.com/c4erries/max_bot/internal/appbot/mocks"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestServiceRunProcessesCommand(t *testing.T) {
	t.Parallel()

	updatesCh := make(chan schemes.UpdateInterface, 1)
	updatesCh <- newTestUpdate("/ping")
	close(updatesCh)

	updates := mocks.NewUpdatesProvider(t)
	sender := mocks.NewMessageSender(t)

	updates.EXPECT().
		GetUpdates(mock.Anything).
		Return(updatesCh).
		Once()

	sender.EXPECT().
		Send(mock.Anything, mock.AnythingOfType("*maxbot.Message")).
		Return("ok", nil).
		Once()

	service := &Service{
		log:      zerolog.New(io.Discard),
		updates:  updates,
		sender:   sender,
		commands: make(map[string]commandEntry),
	}

	service.RegisterCommand(Command{
		Name: "ping",
		Handler: func(ctx context.Context, msg *MessageContext) error {
			return msg.ReplyText(ctx, "pong")
		},
	})

	err := service.Run(context.Background())
	require.NoError(t, err)
}

func TestServiceRunFallbackHandler(t *testing.T) {
	t.Parallel()

	updatesCh := make(chan schemes.UpdateInterface, 1)
	updatesCh <- newTestUpdate("hello there")
	close(updatesCh)

	updates := mocks.NewUpdatesProvider(t)
	sender := mocks.NewMessageSender(t)

	updates.EXPECT().
		GetUpdates(mock.Anything).
		Return(updatesCh).
		Once()

	sender.EXPECT().
		Send(mock.Anything, mock.AnythingOfType("*maxbot.Message")).
		Return("ok", nil).
		Once()

	service := &Service{
		log:      zerolog.New(io.Discard),
		updates:  updates,
		sender:   sender,
		commands: make(map[string]commandEntry),
	}

	service.RegisterMessageHandler(func(ctx context.Context, msg *MessageContext) error {
		return msg.ReplyText(ctx, "got it")
	})

	err := service.Run(context.Background())
	require.NoError(t, err)
}
