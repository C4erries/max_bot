package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// paymentKind описывает тип платежа (общежитие/обучение).
type paymentKind string

const (
	paymentKindDorm    paymentKind = "dorm"
	paymentKindTuition paymentKind = "tuition"
)

// paymentStatus содержит информацию о необходимости оплат.
type paymentStatus struct {
	NeedDorm    bool `json:"need_dorm"`
	NeedTuition bool `json:"need_tuition"`
}

// paymentBackend определяет контракт общения с бекендом по оплатам.
type paymentBackend interface {
	FetchStatus(ctx context.Context, userID int64) (paymentStatus, error)
	CreateLink(ctx context.Context, userID int64, kind paymentKind) (string, error)
}

// paymentService инкапсулирует бизнес-логику проверки и создания ссылок.
type paymentService struct {
	backend paymentBackend
}

func newPaymentService(baseURL string, log zerolog.Logger) (*paymentService, error) {
	var backend paymentBackend
	var err error
	if strings.TrimSpace(baseURL) == "" {
		backend = newStubPaymentBackend()
	} else {
		backend, err = newHTTPPaymentBackend(baseURL, log)
		if err != nil {
			return nil, err
		}
	}
	return &paymentService{backend: backend}, nil
}

func (s *paymentService) Status(ctx context.Context, userID int64) (paymentStatus, error) {
	if s.backend == nil {
		return paymentStatus{}, fmt.Errorf("payment backend is not configured")
	}
	return s.backend.FetchStatus(ctx, userID)
}

func (s *paymentService) Link(ctx context.Context, userID int64, kind paymentKind) (string, error) {
	if s.backend == nil {
		return "", fmt.Errorf("payment backend is not configured")
	}
	return s.backend.CreateLink(ctx, userID, kind)
}

// httpPaymentBackend реализует запросы к внешнему API.
type httpPaymentBackend struct {
	baseURL string
	client  *http.Client
	log     zerolog.Logger
}

func newHTTPPaymentBackend(baseURL string, log zerolog.Logger) (*httpPaymentBackend, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return nil, fmt.Errorf("payment backend: base url is empty")
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("payment backend: parse base url: %w", err)
	}
	u.RawQuery = ""
	u.Fragment = ""
	clean := strings.TrimRight(u.String(), "/")
	if clean == "" {
		return nil, fmt.Errorf("payment backend: resolved base url is empty")
	}
	return &httpPaymentBackend{
		baseURL: clean,
		client:  &http.Client{Timeout: 10 * time.Second},
		log:     log.With().Str("component", "payments").Logger(),
	}, nil
}

func (b *httpPaymentBackend) FetchStatus(ctx context.Context, userID int64) (paymentStatus, error) {
	endpoint := fmt.Sprintf("%s/api/payments/status?user_id=%d", b.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return paymentStatus{}, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return paymentStatus{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return paymentStatus{}, fmt.Errorf("payment status request failed: %s", resp.Status)
	}

	var status paymentStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return paymentStatus{}, err
	}
	return status, nil
}

func (b *httpPaymentBackend) CreateLink(ctx context.Context, userID int64, kind paymentKind) (string, error) {
	body := map[string]interface{}{
		"user_id": userID,
		"kind":    string(kind),
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/payments/link", b.baseURL), &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("payment link request failed: %s", resp.Status)
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.URL == "" {
		// Возвращаем заглушку, чтобы MVP продолжил работу даже без реального URL.
		result.URL = fmt.Sprintf("https://pay.example/%s/%d", kind, userID)
	}
	return result.URL, nil
}
