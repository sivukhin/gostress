package gostress

import (
	"context"
	"go.uber.org/zap"
	"time"
)

type (
	Worker struct {
		WorkerId Id
		Shutdown chan struct{}
	}
)

func (w Worker) Run(work <-chan Id, timeout time.Duration, f StressFn, logger *zap.SugaredLogger) {
	logger.Infof("worker[%v]: created", w.WorkerId)
	for {
		select {
		case id := <-work:
			timer := time.NewTimer(2 * timeout)
			finish := make(chan struct{}, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				startTime := time.Now()
				err := f(RequestContext{Ctx: ctx, Id: id, Logger: logger.With(zap.Int64("worker", int64(w.WorkerId)))})
				finish <- struct{}{}
				status := "success"
				if err != nil {
					status = "error"
					ErrorsCounter.Inc()
					logger.Errorf("worker[%v]: request finished with error: %v", w.WorkerId, err)
				}
				RequestLatency.WithLabelValues(status).Observe(time.Since(startTime).Seconds())
			}()
			select {
			case <-finish:
				continue
			case <-timer.C:
				logger.Errorf("worker[%v]: request %v seems to stuck (didn't finished within %v), killing worker", w.WorkerId, 2*timeout, id)
				return
			}
		case <-w.Shutdown:
			logger.Errorf("worker[%v]: shutdown requested, killing worker", w.WorkerId)
			return
		}
	}
}
