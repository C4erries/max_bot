package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Notifier описывает возможность отправлять пользователю с указанным идентификатором текстовое сообщение.
type Notifier interface {
	NotifyUser(ctx context.Context, userID int64, text string) error
	NotifyDocumentReady(ctx context.Context, userID int64) error
	NotifyTuitionPaymentReminder(ctx context.Context, userID int64) error
}

// Server - минимальный HTTP-API, который проксирует уведомления в сервис бота.
type Server struct {
	addr      string
	notifier  Notifier
	log       zerolog.Logger
	authToken string
}

// New создаёт HTTP-сервер, привязанный к указанному адресу.
func New(address string, notifier Notifier, authToken string, log zerolog.Logger) *Server {
	if notifier == nil {
		panic("httpserver: notifier is nil")
	}
	authToken = strings.TrimSpace(authToken)
	if address == "" {
		address = ":8080"
	}

	return &Server{
		addr:      address,
		notifier:  notifier,
		log:       log.With().Str("component", "httpserver").Logger(),
		authToken: authToken,
	}
}

// Run регистрирует обработчики и обслуживает входящие HTTP-запросы.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /notify/bulk", s.guard(s.handleNotifyBulk))
	mux.HandleFunc("POST /notify/", s.guard(s.handleNotify))
	mux.HandleFunc("POST /notify/ready/", s.guard(s.handleNotifyReady))
	mux.HandleFunc("POST /notify/payment/tuition/", s.guard(s.handleNotifyTuitionReminder))

	server := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserID(r.URL.Path, "/notify/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req notifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	if err := s.notifier.NotifyUser(r.Context(), userID, req.Text); err != nil {
		s.log.Error().Err(err).Int64("user_id", userID).Msg("failed to notify user")
		writeError(w, http.StatusInternalServerError, "failed to deliver notification")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleNotifyReady(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserID(r.URL.Path, "/notify/ready/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.notifier.NotifyDocumentReady(r.Context(), userID); err != nil && err.Error() != "" {
		s.log.Error().Err(err).Int64("user_id", userID).Msg("failed to notify ready document")
		writeError(w, http.StatusInternalServerError, "failed to deliver notification")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleNotifyTuitionReminder(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserID(r.URL.Path, "/notify/payment/tuition/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.notifier.NotifyTuitionPaymentReminder(r.Context(), userID); err != nil {
		s.log.Error().Err(err).Int64("user_id", userID).Msg("failed to send tuition payment reminder")
		writeError(w, http.StatusInternalServerError, "failed to deliver notification")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleNotifyBulk(w http.ResponseWriter, r *http.Request) {
	var req bulkNotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.SenderID <= 0 {
		writeError(w, http.StatusBadRequest, "sender_id must be a positive integer")
		return
	}
	if len(req.UserIDs) == 0 {
		writeError(w, http.StatusBadRequest, "user_ids must contain at least one recipient")
		return
	}

	unique := make([]int64, 0, len(req.UserIDs))
	seen := make(map[int64]struct{}, len(req.UserIDs))
	for idx, userID := range req.UserIDs {
		if userID <= 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("user_ids[%d] must be a positive integer", idx))
			return
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		unique = append(unique, userID)
	}

	for _, userID := range unique {
		if err := s.notifier.NotifyUser(r.Context(), userID, req.Text); err != nil {
			s.log.Error().
				Err(err).
				Int64("sender_id", req.SenderID).
				Int64("user_id", userID).
				Msg("failed to deliver broadcast notification")
			writeError(w, http.StatusInternalServerError, "failed to deliver notification")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "sent",
		"recipients": len(unique),
	})
}

type notifyRequest struct {
	Text string `json:"text"`
}

type bulkNotifyRequest struct {
	Text     string  `json:"text"`
	SenderID int64   `json:"sender_id"`
	UserIDs  []int64 `json:"user_ids"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func parseUserID(path, prefix string) (int64, error) {
	idPart := strings.TrimPrefix(path, prefix)
	idPart = strings.Trim(idPart, "/")
	if idPart == "" {
		return 0, fmt.Errorf("user id is required")
	}

	userID, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("user id must be a positive integer")
	}

	return userID, nil
}

func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	if s.authToken == "" {
		return next
	}
	return s.withAuth(next)
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			s.log.Warn().Str("path", r.URL.Path).Msg("missing auth token")
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		token := strings.TrimSpace(authz[len("Bearer "):])
		if token != s.authToken {
			s.log.Warn().Str("path", r.URL.Path).Msg("invalid auth token")
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		next(w, r)
	}
}
