package appbot

import (
	"context"
	"io"
	"testing"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "spaces", input: "   /Ping  ", want: "ping"},
		{name: "without slash", input: "help", want: "help"},
		{name: "uppercase", input: "/START", want: "start"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, normalizeCommand(tt.input))
		})
	}
}

func TestMessageContextParsing(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	update := newTestUpdate("/Ping user1 user2")

	msgCtx := newMessageContext(svc, update)

	require.Equal(t, "/Ping user1 user2", msgCtx.Text())
	require.Equal(t, "ping", msgCtx.Command())
	require.Equal(t, []string{"user1", "user2"}, msgCtx.Args())
	require.Equal(t, update.GetUserID(), msgCtx.SenderID())
	require.Equal(t, update.GetChatID(), msgCtx.ChatID())
	require.Equal(t, update.Message.Sender.Name, msgCtx.SenderName())
}

func TestServiceHandleMessage(t *testing.T) {
	t.Parallel()

	t.Run("command handler wins", func(t *testing.T) {
		t.Parallel()

		svc := newTestService()
		commandCalled := false
		fallbackCalled := false

		svc.RegisterCommand(Command{
			Name: "echo",
			Handler: func(ctx context.Context, msg *MessageContext) error {
				commandCalled = true
				return nil
			},
		})
		svc.RegisterMessageHandler(func(ctx context.Context, msg *MessageContext) error {
			fallbackCalled = true
			return nil
		})

		err := svc.handleMessage(context.Background(), newTestUpdate("/echo hi"))
		require.NoError(t, err)
		require.True(t, commandCalled)
		require.False(t, fallbackCalled)
	})

	t.Run("fallback fires for plain text", func(t *testing.T) {
		t.Parallel()

		svc := newTestService()
		fallbackCalled := false

		svc.RegisterMessageHandler(func(ctx context.Context, msg *MessageContext) error {
			fallbackCalled = true
			return nil
		})

		err := svc.handleMessage(context.Background(), newTestUpdate("hello"))
		require.NoError(t, err)
		require.True(t, fallbackCalled)
	})

	t.Run("session handler intercepts before commands", func(t *testing.T) {
		t.Parallel()

		svc := newTestService()
		sessionCalled := false
		commandCalled := false

		svc.SetSessionState(42, SessionState{Step: "pending"})
		svc.RegisterSessionHandler("pending", func(ctx context.Context, msg *MessageContext, state SessionState) error {
			sessionCalled = true
			require.Equal(t, "pending", state.Step)
			msg.ClearSessionState()
			return nil
		})

		svc.RegisterCommand(Command{
			Name: "hello",
			Handler: func(ctx context.Context, msg *MessageContext) error {
				commandCalled = true
				return nil
			},
		})

		err := svc.handleMessage(context.Background(), newTestUpdate("/hello"))
		require.NoError(t, err)
		require.True(t, sessionCalled)
		require.False(t, commandCalled)
	})
}

func TestServiceHandleCallback(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	callbackCalled := false

	svc.RegisterCallbackHandler(func(ctx context.Context, cb *CallbackContext) error {
		callbackCalled = true
		require.Equal(t, "payload", cb.Payload())
		return nil
	})

	err := svc.handleCallback(context.Background(), newTestCallbackUpdate("payload"))
	require.NoError(t, err)
	require.True(t, callbackCalled)
}

func TestServiceHandleBotStarted(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	startCalled := false

	svc.RegisterBotStartedHandler(func(ctx context.Context, start *BotStartedContext) error {
		startCalled = true
		require.Equal(t, int64(42), start.UserID())
		require.Equal(t, int64(7), start.ChatID())
		return nil
	})

	err := svc.handleBotStarted(context.Background(), newTestBotStartedUpdate())
	require.NoError(t, err)
	require.True(t, startCalled)
}

func newTestService() *Service {
	return &Service{
		log:             zerolog.New(io.Discard),
		commands:        make(map[string]commandEntry),
		sessions:        make(map[int64]SessionState),
		sessionHandlers: make(map[string]SessionHandler),
	}
}

func newTestUpdate(text string) *schemes.MessageCreatedUpdate {
	return &schemes.MessageCreatedUpdate{
		Message: schemes.Message{
			Sender: schemes.User{
				UserId: 42,
				Name:   "Tester",
			},
			Recipient: schemes.Recipient{
				ChatId:   7,
				ChatType: schemes.DIALOG,
			},
			Body: schemes.MessageBody{
				Text: text,
			},
		},
	}
}

func newTestCallbackUpdate(payload string) *schemes.MessageCallbackUpdate {
	return &schemes.MessageCallbackUpdate{
		Callback: schemes.Callback{
			CallbackID: "cb",
			Payload:    payload,
			User: schemes.User{
				UserId: 42,
				Name:   "Tester",
			},
		},
		Message: &schemes.Message{
			Recipient: schemes.Recipient{
				ChatId: 7,
			},
		},
	}
}

func newTestBotStartedUpdate() *schemes.BotStartedUpdate {
	return &schemes.BotStartedUpdate{
		ChatId: 7,
		User: schemes.User{
			UserId: 42,
			Name:   "Tester",
		},
	}
}
