package gostress

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"time"
)

type (
	Id             int64
	RequestContext struct {
		Id     Id
		Ctx    context.Context
		Logger *zap.SugaredLogger
	}
)

func (p *LoadParams) String() string {
	return fmt.Sprintf("{rps: %v, workers: %v, duration: %v}", p.Rps, p.Workers, p.Duration)
}

func interpolate(start, end LoadParams, d time.Duration) LoadParams {
	f := float64(d.Nanoseconds()) / float64(start.Duration.Nanoseconds())
	return LoadParams{
		Rps:     start.Rps + int(float64(end.Rps-start.Rps)*f),
		Workers: start.Workers + int(float64(end.Workers-start.Workers)*f),
	}
}
