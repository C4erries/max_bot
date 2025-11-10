package httpserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type stubNotifier struct {
	userID int64
	text   string
	err    error
	called bool
}

func (n *stubNotifier) NotifyUser(_ context.Context, userID int64, text string) error {
	n.called = true
	n.userID = userID
	n.text = text
	return n.err
}

func newTestServer(t *testing.T) (*Server, *stubNotifier) {
	t.Helper()
	notifier := &stubNotifier{}
	srv := New(":0", notifier, zerolog.New(io.Discard))
	return srv, notifier
}

func TestHandleNotifySuccess(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/notify/42", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	srv.handleNotify(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, notifier.called)
	require.Equal(t, int64(42), notifier.userID)
	require.Equal(t, "hello", notifier.text)
}

func TestHandleNotifyValidation(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/notify/abc", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	srv.handleNotify(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, notifier.called)

	req = httptest.NewRequest(http.MethodPost, "/notify/1", strings.NewReader(`{"text":""}`))
	rec = httptest.NewRecorder()
	srv.handleNotify(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, notifier.called)
}

func TestHandleNotifyNotifierError(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)
	notifier.err = errors.New("boom")

	req := httptest.NewRequest(http.MethodPost, "/notify/1", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	srv.handleNotify(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.True(t, notifier.called)
}
