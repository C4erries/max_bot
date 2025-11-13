package httpserver

import (
	"context"
	"encoding/json"
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
	notifyUserID  int64
	readyUserID   int64
	tuitionUserID int64
	text          string
	notifyErr     error
	readyErr      error
	tuitionErr    error
	calledNotify  bool
	calledReady   bool
	calledTuition bool
	users         []int64
}

func (n *stubNotifier) NotifyUser(_ context.Context, userID int64, text string) error {
	n.calledNotify = true
	n.notifyUserID = userID
	n.users = append(n.users, userID)
	n.text = text
	return n.notifyErr
}

func (n *stubNotifier) NotifyDocumentReady(_ context.Context, userID int64) error {
	n.calledReady = true
	n.readyUserID = userID
	return n.readyErr
}

func (n *stubNotifier) NotifyTuitionPaymentReminder(_ context.Context, userID int64) error {
	n.calledTuition = true
	n.tuitionUserID = userID
	return n.tuitionErr
}

func newTestServer(t *testing.T) (*Server, *stubNotifier) {
	return newTestServerWithToken(t, "test-token")
}

func newTestServerWithToken(t *testing.T, token string) (*Server, *stubNotifier) {
	t.Helper()
	notifier := &stubNotifier{}
	srv := New(":0", notifier, token, zerolog.New(io.Discard))
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

func TestHandleNotifyBulkSuccess(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)
	body := `{"text":" bulk message ","sender_id":12,"user_ids":[1,2,2]}`

	req := httptest.NewRequest(http.MethodPost, "/notify/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()

	srv.handleNotifyBulk(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, notifier.calledNotify)
	require.Equal(t, []int64{1, 2}, notifier.users)
	require.Equal(t, "bulk message", notifier.text)

	var resp struct {
		Status     string `json:"status"`
		Recipients int    `json:"recipients"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "sent", resp.Status)
	require.Equal(t, 2, resp.Recipients)
}

func TestHandleNotifyBulkValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		body string
	}{
		{"invalid json", `{"text":`},
		{"empty text", `{"text":" ","sender_id":1,"user_ids":[1]}`},
		{"missing sender", `{"text":"hi","user_ids":[1]}`},
		{"invalid sender", `{"text":"hi","sender_id":0,"user_ids":[1]}`},
		{"no recipients", `{"text":"hi","sender_id":1,"user_ids":[]}`},
		{"invalid recipient", `{"text":"hi","sender_id":1,"user_ids":[-2,3]}`},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, notifier := newTestServer(t)

			req := httptest.NewRequest(http.MethodPost, "/notify/bulk", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			srv.handleNotifyBulk(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.False(t, notifier.calledNotify)
		})
	}
}

func TestHandleNotifyBulkNotifierError(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)
	notifier.notifyErr = errors.New("boom")

	req := httptest.NewRequest(http.MethodPost, "/notify/bulk", strings.NewReader(`{"text":"hi","sender_id":3,"user_ids":[10,11]}`))
	rec := httptest.NewRecorder()

	srv.handleNotifyBulk(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.True(t, notifier.calledNotify)
	require.Equal(t, []int64{10}, notifier.users)
}

func TestWithAuth(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t)
	handler := srv.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/notify/1", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/notify/1", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	handler(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/notify/1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	handler(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGuardWithoutToken(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServerWithToken(t, "")
	handler := srv.guard(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/notify/1", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
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

func TestHandleNotifyTuitionReminderSuccess(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/notify/payment/tuition/33", nil)
	rec := httptest.NewRecorder()

	srv.handleNotifyTuitionReminder(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, notifier.calledTuition)
	require.Equal(t, int64(33), notifier.tuitionUserID)
}

func TestHandleNotifyTuitionReminderValidation(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/notify/payment/tuition/", nil)
	rec := httptest.NewRecorder()

	srv.handleNotifyTuitionReminder(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, notifier.calledTuition)
}

func TestHandleNotifyTuitionReminderNotifierError(t *testing.T) {
	t.Parallel()

	srv, notifier := newTestServer(t)
	notifier.tuitionErr = errors.New("boom")

	req := httptest.NewRequest(http.MethodPost, "/notify/payment/tuition/77", nil)
	rec := httptest.NewRecorder()

	srv.handleNotifyTuitionReminder(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.True(t, notifier.calledTuition)
}
