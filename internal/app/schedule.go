package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c4erries/max_bot/internal/backend"
)

type scheduleService struct {
	backend backend.Schedule
	now     func() time.Time
}

func newScheduleService(client backend.Schedule) (*scheduleService, error) {
	if client == nil {
		return nil, fmt.Errorf("schedule backend is nil")
	}
	return &scheduleService{
		backend: client,
		now:     time.Now,
	}, nil
}

func (s *scheduleService) Today(ctx context.Context, userID int64) (string, error) {
	lessons, weekStart, err := s.fetchWeek(ctx, userID)
	if err != nil {
		return "", err
	}

	today := normalizeWeekday(s.now())
	todayLessons := filterLessonsByDay(lessons, today)
	if len(todayLessons) == 0 && len(lessons) > 0 {
		todayLessons = limitLessons(lessons, 4)
	}
	if len(todayLessons) == 0 {
		return "Сегодня пар нет — можно заняться своими делами 👌", nil
	}

	sortLessons(todayLessons)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Сегодня (%s) занятия:\n\n", weekdayTitles[today]))
	for i, lesson := range todayLessons {
		b.WriteString(formatLessonLine(i+1, lesson))
		b.WriteString("\n")
	}
	b.WriteString("\nЧтобы посмотреть всю неделю, нажми «На эту неделю».")

	_ = weekStart // оставлено для возможного будущего использования (например, вывода даты)
	return strings.TrimSpace(b.String()), nil
}

func (s *scheduleService) Week(ctx context.Context, userID int64) (string, error) {
	lessons, weekStart, err := s.fetchWeek(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(lessons) == 0 {
		return "На этой неделе занятий нет — отдыхайте и набирайтесь сил ☀️", nil
	}

	grouped := make(map[time.Weekday][]backend.ScheduleLesson)
	var withoutDay []backend.ScheduleLesson
	for _, lesson := range lessons {
		if day, ok := detectWeekday(lesson); ok {
			grouped[day] = append(grouped[day], lesson)
			continue
		}
		withoutDay = append(withoutDay, lesson)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"Неделя %s — %s:\n\n",
		weekStart.Format("02.01"),
		weekStart.AddDate(0, 0, 6).Format("02.01"),
	))

	for _, day := range weekOrder {
		dayLessons := grouped[day]
		if len(dayLessons) == 0 {
			continue
		}
		sortLessons(dayLessons)
		b.WriteString(weekdayTitles[day])
		b.WriteString(":\n")
		for i, lesson := range dayLessons {
			b.WriteString(formatLessonLine(i+1, lesson))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(withoutDay) > 0 {
		sortLessons(withoutDay)
		b.WriteString("Без указания дня:\n")
		for i, lesson := range withoutDay {
			b.WriteString(formatLessonLine(i+1, lesson))
			b.WriteString("\n")
		}
	}

	return strings.TrimSpace(b.String()), nil
}

func (s *scheduleService) fetchWeek(ctx context.Context, userID int64) ([]backend.ScheduleLesson, time.Time, error) {
	start := startOfWeek(s.now())
	lessons, err := s.backend.List(ctx, userID, &start)
	if err != nil {
		return nil, time.Time{}, err
	}
	return lessons, start, nil
}

func startOfWeek(t time.Time) time.Time {
	weekday := normalizeWeekday(t)
	diff := int(weekday - time.Monday)
	if diff < 0 {
		diff = 6
	}
	day := t.AddDate(0, 0, -diff)
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, t.Location())
}

func normalizeWeekday(t time.Time) time.Weekday {
	wd := t.Weekday()
	if wd == time.Sunday {
		return time.Sunday
	}
	return wd
}

func filterLessonsByDay(list []backend.ScheduleLesson, day time.Weekday) []backend.ScheduleLesson {
	var out []backend.ScheduleLesson
	for _, lesson := range list {
		if lessonDay, ok := detectWeekday(lesson); ok && lessonDay == day {
			out = append(out, lesson)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func detectWeekday(lesson backend.ScheduleLesson) (time.Weekday, bool) {
	if lesson.Weekday != "" {
		if wd, ok := parseWeekday(lesson.Weekday); ok {
			return wd, true
		}
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01-02 15:04:05",
		"2006/01/02",
	}
	for _, layout := range layouts {
		if lesson.Date == "" {
			break
		}
		if parsed, err := time.Parse(layout, lesson.Date); err == nil {
			return normalizeWeekday(parsed), true
		}
	}
	return 0, false
}

var weekdayTitles = map[time.Weekday]string{
	time.Monday:    "Понедельник",
	time.Tuesday:   "Вторник",
	time.Wednesday: "Среда",
	time.Thursday:  "Четверг",
	time.Friday:    "Пятница",
	time.Saturday:  "Суббота",
	time.Sunday:    "Воскресенье",
}

var weekdayAliases = map[string]time.Weekday{
	"monday":      time.Monday,
	"понедельник": time.Monday,
	"tuesday":     time.Tuesday,
	"вторник":     time.Tuesday,
	"wednesday":   time.Wednesday,
	"среда":       time.Wednesday,
	"thursday":    time.Thursday,
	"четверг":     time.Thursday,
	"friday":      time.Friday,
	"пятница":     time.Friday,
	"saturday":    time.Saturday,
	"суббота":     time.Saturday,
	"sunday":      time.Sunday,
	"воскресенье": time.Sunday,
}

var weekOrder = []time.Weekday{
	time.Monday,
	time.Tuesday,
	time.Wednesday,
	time.Thursday,
	time.Friday,
	time.Saturday,
	time.Sunday,
}

func parseWeekday(value string) (time.Weekday, bool) {
	wd, ok := weekdayAliases[strings.ToLower(strings.TrimSpace(value))]
	return wd, ok
}

func sortLessons(list []backend.ScheduleLesson) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Weekday != list[j].Weekday {
			di, okI := parseWeekday(list[i].Weekday)
			dj, okJ := parseWeekday(list[j].Weekday)
			if okI && okJ && di != dj {
				return di < dj
			}
		}
		if list[i].PairNo != list[j].PairNo {
			return list[i].PairNo < list[j].PairNo
		}
		return list[i].Subject < list[j].Subject
	})
}

func formatLessonLine(index int, lesson backend.ScheduleLesson) string {
	timeText := strings.TrimSpace(lesson.Time)
	if timeText == "" {
		if lesson.PairNo > 0 {
			timeText = fmt.Sprintf("пара #%d", lesson.PairNo)
		} else {
			timeText = "время не указано"
		}
	}
	subject := safeText(lesson.Subject, "Предмет не указан")
	room := safeText(lesson.Room, "")
	teacher := safeText(lesson.Teacher, "")

	var meta []string
	if room != "" {
		meta = append(meta, room)
	}
	if teacher != "" {
		meta = append(meta, fmt.Sprintf("преподаватель %s", teacher))
	}
	if len(lesson.Groups) > 0 {
		meta = append(meta, fmt.Sprintf("группы: %s", strings.Join(lesson.Groups, ", ")))
	}

	details := ""
	if len(meta) > 0 {
		details = fmt.Sprintf(" (%s)", strings.Join(meta, "; "))
	}
	return fmt.Sprintf("%d) %s · %s%s", index, timeText, subject, details)
}

func safeText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func limitLessons(list []backend.ScheduleLesson, max int) []backend.ScheduleLesson {
	if max <= 0 || len(list) == 0 {
		return nil
	}
	copyList := append([]backend.ScheduleLesson(nil), list...)
	sortLessons(copyList)
	if len(copyList) > max {
		copyList = copyList[:max]
	}
	return copyList
}
