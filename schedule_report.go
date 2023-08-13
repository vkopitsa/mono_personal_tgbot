package main

import (
	"context"
	"errors"

	"github.com/adhocore/gronx"
	"github.com/adhocore/gronx/pkg/tasker"
)

type ScheduleReport struct {
	cron         *gronx.Gronx
	Taskr        *tasker.Tasker
	scheduleTime string
}

func NewScheduleReport(scheduleTime string) (*ScheduleReport, error) {
	gron := gronx.New()
	if !gron.IsValid(scheduleTime) {
		return nil, errors.New("incorrect expression")
	}

	taskr := tasker.New(tasker.Option{
		Verbose: true,
		Tz:      "Europe/Kyiv",
	})

	return &ScheduleReport{
		cron:         &gron,
		Taskr:        taskr,
		scheduleTime: scheduleTime,
	}, nil
}

func (s *ScheduleReport) Start(f func(ctx context.Context) (int, error)) {
	// add task to run every minute
	s.Taskr.Task(s.scheduleTime, f)

	s.Taskr.Run()
}
