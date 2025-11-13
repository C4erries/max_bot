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
	notifyUserID int64
	readyUserID  int64
	text         string
	notifyErr    error
	readyErr     error
	calledNotify bool
	calledReady  bool
}

func (n *stubNotifier) NotifyUser(_ context.Context, userID int64, text string) error {
	n.calledNotify = true
	n.notifyUserID = userID
	n.text = text
	return n.notifyErr
}

func (n *stubNotifier) NotifyDocumentReady(_ context.Context, userID int64) error {
	n.calledReady = true
	n.readyUserID = userID
	return n.readyErr
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
	require.True(t, notifier.calledNotify)
	require.Equal(t, int64(42), notifier.notifyUserID)
	require.Equal(t, "hello", notifier.text)
}

func TestHandleNotifyValidation(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/notify/abc", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	srv.handleNotify(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, notifier.calledNotify)

	req = httptest.NewRequest(http.MethodPost, "/notify/1", strings.NewReader(`{"text":""}`))
	rec = httptest.NewRecorder()
	srv.handleNotify(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, notifier.calledNotify)
}

func TestHandleNotifyNotifierError(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)
	notifier.notifyErr = errors.New("boom")

	req := httptest.NewRequest(http.MethodPost, "/notify/1", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	srv.handleNotify(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.True(t, notifier.calledNotify)
}

func TestHandleNotifyReadySuccess(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/notify/ready/21", nil)
	rec := httptest.NewRecorder()

	srv.handleNotifyReady(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, notifier.calledReady)
	require.Equal(t, int64(21), notifier.readyUserID)
}

func TestHandleNotifyReadyValidation(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/notify/ready/", nil)
	rec := httptest.NewRecorder()

	srv.handleNotifyReady(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, notifier.calledReady)
}

func TestHandleNotifyReadyNotifierError(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)
	notifier.readyErr = errors.New("boom")

	req := httptest.NewRequest(http.MethodPost, "/notify/ready/7", nil)
	rec := httptest.NewRecorder()

	srv.handleNotifyReady(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.True(t, notifier.calledReady)
}
