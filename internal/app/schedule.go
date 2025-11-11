package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// ScheduleLesson описывает одну пару из расписания.
type ScheduleLesson struct {
	Time     string `json:"time"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

type scheduleBackend interface {
	Today(ctx context.Context, userID int64) ([]ScheduleLesson, error)
}

type scheduleService struct {
	backend scheduleBackend
}

func newScheduleService(baseURL string, log zerolog.Logger) (*scheduleService, error) {
	var backend scheduleBackend
	var err error
	if strings.TrimSpace(baseURL) == "" {
		backend = newStubScheduleBackend()
	} else {
		backend, err = newHTTPScheduleBackend(baseURL, log)
		if err != nil {
			return nil, err
		}
	}
	return &scheduleService{backend: backend}, nil
}

func (s *scheduleService) Today(ctx context.Context, userID int64) (string, error) {
	if s.backend == nil {
		return "", fmt.Errorf("schedule backend is not configured")
	}
	lessons, err := s.backend.Today(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(lessons) == 0 {
		return "На сегодня пар нет — отдыхайте!", nil
	}
	var b strings.Builder
	b.WriteString("Расписание на сегодня:\n")
	for i, lesson := range lessons {
		fmt.Fprintf(&b, "%d) %s — %s (%s)\n", i+1, lesson.Time, lesson.Title, lesson.Location)
	}
	return b.String(), nil
}

type httpScheduleBackend struct {
	baseURL string
	client  *http.Client
	log     zerolog.Logger
}

func newHTTPScheduleBackend(baseURL string, log zerolog.Logger) (*httpScheduleBackend, error) {
	base := strings.TrimSpace(baseURL)
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("schedule backend: parse base url: %w", err)
	}
	u.RawQuery = ""
	u.Fragment = ""
	clean := strings.TrimRight(u.String(), "/")
	if clean == "" {
		return nil, fmt.Errorf("schedule backend: resolved base url is empty")
	}
	return &httpScheduleBackend{
		baseURL: clean,
		client:  &http.Client{Timeout: 10 * time.Second},
		log:     log.With().Str("component", "schedule").Logger(),
	}, nil
}

func (b *httpScheduleBackend) Today(ctx context.Context, userID int64) ([]ScheduleLesson, error) {
	endpoint := fmt.Sprintf("%s/api/schedule/today?user_id=%d", b.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("schedule request failed: %s", resp.Status)
	}

	var lessons []ScheduleLesson
	if err := json.NewDecoder(resp.Body).Decode(&lessons); err != nil {
		return nil, err
	}
	return lessons, nil
}
