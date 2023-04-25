package main

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"time"
)

type (
	LoadSchedule []LoadParams
	LoadParams   struct {
		Rps      int
		Workers  int
		Duration time.Duration
	}
	Runner struct{ Id Id }
)

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Trigger(work chan<- Id) (Id, bool) {
	id := r.Id
	r.Id++
	select {
	case work <- id:
		return id, true
	default:
		return id, false
	}
}

func (r *Runner) RunSimpleSchedule(ctx context.Context, start, end LoadParams, pool *WorkerPool, logger *zap.SugaredLogger) {
	logger.Infof("start simple schedule: start=%v, end=%v", start, end)
	startTime := time.Now()
	for !Finished(ctx) {
		elapsed := time.Since(startTime)
		if elapsed >= start.Duration {
			break
		}
		currentParams := interpolate(start, end, elapsed)
		pool.Adjust(currentParams.Workers)
		ExpectedRpsGauge.Set(float64(currentParams.Rps))
		ExpectedWorkersGauge.Set(float64(currentParams.Workers))
		CurrentWorkersGauge.Set(float64(len(pool.Workers)))
		id, ok := r.Trigger(pool.Work)
		if ok {
			SentRequestCounter.Inc()
		} else {
			logger.Warnf("request %v was skipped because there were no free worker", id)
			SkippedRequestCounter.Inc()
		}
		time.Sleep(time.Duration(1.0 / float64(currentParams.Rps) * float64(time.Second)))
	}
}

func (r *Runner) RunSchedule(ctx context.Context, schedule LoadSchedule, pool *WorkerPool, logger *zap.SugaredLogger) error {
	logger.Infof("start schedule: %v", schedule)
	for i := 0; i < len(schedule) && !Finished(ctx); i++ {
		start, end := schedule[i], schedule[i]
		if i+1 < len(schedule) {
			end = schedule[i+1]
		}
		r.RunSimpleSchedule(ctx, start, end, pool, logger)
	}
	if Finished(ctx) {
		return fmt.Errorf("forcibly finish schedule: context was cancelled")
	}
	return nil
}
