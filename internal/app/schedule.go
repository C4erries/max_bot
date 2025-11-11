package app

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/c4erries/max_bot/internal/backend"
)

type scheduleService struct {
	backend backend.Schedule
}

func newScheduleService(client backend.Schedule) (*scheduleService, error) {
	if client == nil {
		return nil, fmt.Errorf("schedule backend is nil")
	}
	return &scheduleService{backend: client}, nil
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
		return "<b>На сегодня пар нет — отдыхайте!</b>", nil
	}

	var b strings.Builder
	b.WriteString("<b>Расписание на сегодня:</b><br/>")
	for i, lesson := range lessons {
		timeText := html.EscapeString(lesson.Time)
		titleText := html.EscapeString(lesson.Title)
		locationText := html.EscapeString(lesson.Location)
		fmt.Fprintf(&b, "%d) <b>%s</b> — %s", i+1, timeText, titleText)
		if locationText != "" {
			fmt.Fprintf(&b, " <i>(%s)</i>", locationText)
		}
		b.WriteString("<br/>")
	}
	return b.String(), nil
}
