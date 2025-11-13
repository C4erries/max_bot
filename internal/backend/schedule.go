package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// ScheduleLesson описывает одну пару из расписания.
type ScheduleLesson struct {
	ID      string   `json:"id"`
	Subject string   `json:"subject"`
	Teacher string   `json:"teacher"`
	Room    string   `json:"room"`
	PairNo  int      `json:"pair_no"`
	Time    string   `json:"time"`
	Groups  []string `json:"groups"`
	Weekday string   `json:"weekday,omitempty"`
	Date    string   `json:"date,omitempty"`
}

// Schedule инкапсулирует взаимодействие с backend API.
type Schedule interface {
	List(ctx context.Context, userID int64, weekStart *time.Time) ([]ScheduleLesson, error)
}

// NewSchedule возвращает реализацию клиента расписания.
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

func (s *httpSchedule) List(ctx context.Context, userID int64, weekStart *time.Time) ([]ScheduleLesson, error) {
	params := url.Values{}
	params.Set("max_id", strconv.FormatInt(userID, 10))
	if weekStart != nil {
		params.Set("week_start", weekStart.Format("2006-01-02"))
	}

	endpoint := fmt.Sprintf("%s/schedule?%s", s.baseURL, params.Encode())
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

func (stubSchedule) List(_ context.Context, _ int64, _ *time.Time) ([]ScheduleLesson, error) {
	return []ScheduleLesson{
		{
			ID:      "lesson-1",
			Subject: "Линейная алгебра",
			Teacher: "Анна Сергеевна",
			Room:    "ауд. 101",
			PairNo:  1,
			Time:    "08:00 — 09:20",
			Groups:  []string{"ИКБО-01-23"},
			Weekday: "monday",
		},
		{
			ID:      "lesson-2",
			Subject: "Теория вероятностей",
			Teacher: "Павел Михайлович",
			Room:    "ауд. 215",
			PairNo:  2,
			Time:    "09:30 — 10:50",
			Groups:  []string{"ИКБО-01-23"},
			Weekday: "monday",
		},
		{
			ID:      "lesson-3",
			Subject: "Программирование",
			Teacher: "Екатерина Андреевна",
			Room:    "ауд. 305",
			PairNo:  3,
			Time:    "11:10 — 12:30",
			Groups:  []string{"ИКБО-01-23"},
			Weekday: "tuesday",
		},
	}, nil
}

var _ Schedule = (*httpSchedule)(nil)
var _ Schedule = (*stubSchedule)(nil)
