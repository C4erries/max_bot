package app

import (
	"context"
	"fmt"
)

func newStubPaymentBackend() paymentBackend {
	return stubPaymentBackend{}
}

type stubPaymentBackend struct{}

func (stubPaymentBackend) FetchStatus(_ context.Context, _ int64) (paymentStatus, error) {
	return paymentStatus{NeedDorm: true, NeedTuition: false}, nil
}

func (stubPaymentBackend) CreateLink(_ context.Context, userID int64, kind paymentKind) (string, error) {
	return fmt.Sprintf("https://pay.mock/%s/%d", kind, userID), nil
}

func newStubScheduleBackend() scheduleBackend {
	return stubScheduleBackend{}
}

type stubScheduleBackend struct{}

func (stubScheduleBackend) Today(_ context.Context, _ int64) ([]ScheduleLesson, error) {
	return []ScheduleLesson{
		{Time: "08:00 - 09:20", Title: "Информатика", Location: "лаб. 101"},
		{Time: "09:30 - 10:50", Title: "Алгебра", Location: "каб. 215"},
		{Time: "11:10 - 12:30", Title: "Физкультура", Location: "зал ргф"},
		{Time: "12:40 - 14:00", Title: "Геометрия", Location: "ауд. 120"},
	}, nil
}
