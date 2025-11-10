package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Notifier описывает возможность отправлять пользователю с указанным идентификатором текстовое сообщение.
type Notifier interface {
	NotifyUser(ctx context.Context, userID int64, text string) error
}

// Server - минимальный HTTP-API, который проксирует уведомления в сервис бота.
type Server struct {
	addr     string
	notifier Notifier
	log      zerolog.Logger
}

// New создаёт HTTP-сервер, привязанный к указанному адресу.
func New(address string, notifier Notifier, log zerolog.Logger) *Server {
	if notifier == nil {
		panic("httpserver: notifier is nil")
	}

	if address == "" {
		address = ":8080"
	}

	return &Server{
		addr:     address,
		notifier: notifier,
		log:      log.With().Str("component", "httpserver").Logger(),
	}
}

// Run регистрирует обработчики и обслуживает входящие HTTP-запросы.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /notify/", s.handleNotify)

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
	idPart := strings.TrimPrefix(r.URL.Path, "/notify/")
	idPart = strings.Trim(idPart, "/")
	if idPart == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	userID, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "user id must be a positive integer")
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

type notifyRequest struct {
	Text string `json:"text"`
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
