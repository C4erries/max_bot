package backend

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

// PaymentKind описывает тип платежа (общежитие/обучение).
type PaymentKind string

const (
	PaymentKindDorm    PaymentKind = "dorm"
	PaymentKindTuition PaymentKind = "tuition"
)

// PaymentStatus содержит флаги задолженностей пользователя.
type PaymentStatus struct {
	NeedDorm    bool `json:"need_dorm"`
	NeedTuition bool `json:"need_tuition"`
}

// Payments описывает интерфейс взаимодействия с backend для платежей.
type Payments interface {
	Status(ctx context.Context, userID int64) (PaymentStatus, error)
	Link(ctx context.Context, userID int64, kind PaymentKind) (string, error)
}

// NewPayments создает HTTP-клиент или стаб, если baseURL не указан.
func NewPayments(baseURL string, log zerolog.Logger) (Payments, error) {
	if strings.TrimSpace(baseURL) == "" {
		return stubPayments{}, nil
	}
	return newHTTPPayments(baseURL, log)
}

type httpPayments struct {
	baseURL string
	client  *http.Client
	log     zerolog.Logger
}

func newHTTPPayments(baseURL string, log zerolog.Logger) (*httpPayments, error) {
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
	return &httpPayments{
		baseURL: clean,
		client:  &http.Client{Timeout: 10 * time.Second},
		log:     log.With().Str("component", "payments").Logger(),
	}, nil
}

func (p *httpPayments) Status(ctx context.Context, userID int64) (PaymentStatus, error) {
	endpoint := fmt.Sprintf("%s/api/payments/status?user_id=%d", p.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return PaymentStatus{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return PaymentStatus{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return PaymentStatus{}, fmt.Errorf("payment status request failed: %s", resp.Status)
	}

	var status PaymentStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return PaymentStatus{}, err
	}
	return status, nil
}

func (p *httpPayments) Link(ctx context.Context, userID int64, kind PaymentKind) (string, error) {
	body := map[string]interface{}{
		"user_id": userID,
		"kind":    string(kind),
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/payments/link", p.baseURL), &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("payment link request failed: %s", resp.Status)
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.URL == "" {
		// Fallback для MVP, если backend не вернул валидную ссылку.
		result.URL = fmt.Sprintf("https://pay.example/%s/%d", kind, userID)
	}
	return result.URL, nil
}

type stubPayments struct{}

func (stubPayments) Status(_ context.Context, _ int64) (PaymentStatus, error) {
	return PaymentStatus{NeedDorm: true, NeedTuition: false}, nil
}

func (stubPayments) Link(_ context.Context, userID int64, kind PaymentKind) (string, error) {
	return fmt.Sprintf("https://pay.mock/%s/%d", kind, userID), nil
}

var _ Payments = (*httpPayments)(nil)
var _ Payments = (*stubPayments)(nil)
