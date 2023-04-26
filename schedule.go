package gostress

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type (
	LoadSchedule []LoadParams
	LoadParams   struct {
		Rps      int
		Workers  int
		Duration time.Duration
	}
	Runner struct{ Id int64 }
)

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Trigger(work chan<- Id) (Id, bool) {
	id := atomic.AddInt64(&r.Id, 1)
	select {
	case work <- Id(id):
		return Id(id), true
	default:
		return Id(id), false
	}
}

func (r *Runner) RunSimpleSchedule(ctx context.Context, start, end LoadParams, pool *WorkerPool, logger *zap.SugaredLogger) {
	logger.Infof("start simple schedule: start=%v, end=%v", start, end)
	startTime := time.Now()
	maxRps := start.Rps
	if end.Rps > start.Rps {
		maxRps = end.Rps
	}
	const maxRaterRps = 20
	raters, ratersCount := sync.WaitGroup{}, (maxRps+maxRaterRps-1)/maxRaterRps
	for i := 0; i < ratersCount; i++ {
		raters.Add(1)
		go func() {
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
				sleepTime := time.Duration(2 * rand.Float64() / float64(currentParams.Rps) * float64(time.Second) * float64(ratersCount))
				time.Sleep(sleepTime)
			}
			raters.Done()
		}()
	}
	raters.Wait()
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
