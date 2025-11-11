package backend

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

// Schedule описывает интерфейс для получения расписания пользователя.
type Schedule interface {
	Today(ctx context.Context, userID int64) ([]ScheduleLesson, error)
}

// NewSchedule возвращает HTTP-клиент или стаб в зависимости от baseURL.
func NewSchedule(baseURL string, log zerolog.Logger) (Schedule, error) {
	if strings.TrimSpace(baseURL) == "" {
		return stubSchedule{}, nil
	}
	return newHTTPSchedule(baseURL, log)
}

type httpSchedule struct {
	baseURL string
	client  *http.Client
	log     zerolog.Logger
}

func newHTTPSchedule(baseURL string, log zerolog.Logger) (*httpSchedule, error) {
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
	return &httpSchedule{
		baseURL: clean,
		client:  &http.Client{Timeout: 10 * time.Second},
		log:     log.With().Str("component", "schedule").Logger(),
	}, nil
}

func (s *httpSchedule) Today(ctx context.Context, userID int64) ([]ScheduleLesson, error) {
	endpoint := fmt.Sprintf("%s/api/schedule/today?user_id=%d", s.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("schedule request failed: %s", resp.Status)
	}

	var lessons []ScheduleLesson
	if err := json.NewDecoder(resp.Body).Decode(&lessons); err != nil {
		return nil, err
	}
	return lessons, nil
}

type stubSchedule struct{}

func (stubSchedule) Today(_ context.Context, _ int64) ([]ScheduleLesson, error) {
	return []ScheduleLesson{
		{Time: "08:00 – 09:20", Title: "Математика", Location: "Корпус А, 101"},
		{Time: "09:30 – 10:50", Title: "Теория вероятностей", Location: "Корпус А, 215"},
		{Time: "11:10 – 12:30", Title: "История", Location: "Корпус Б, 305"},
	}, nil
}

var _ Schedule = (*httpSchedule)(nil)
var _ Schedule = (*stubSchedule)(nil)
