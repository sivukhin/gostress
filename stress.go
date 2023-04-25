package main

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"net/http"
	"testing"
	"time"
)

type (
	Stress struct {
		Name           string
		Nonce          string
		Workers        *WorkerPool
		Runner         *Runner
		Schedule       LoadSchedule
		ReportInterval time.Duration
		Logger         *zap.SugaredLogger
		MetricsPort    int
	}

	GoStressOptions struct {
		WorkerTimeout  time.Duration
		Schedule       LoadSchedule
		ReportInterval time.Duration
		MetricsPort    int
	}
)

func NewGoStress(t *testing.T, options GoStressOptions, f StressFn) (Stress, func()) {
	registerMetrics(t.Name())
	logger := zaptest.NewLogger(t).Sugar()

	logger.Infof("initialized gostress instance for test %v with timeout %v", t.Name(), options.WorkerTimeout)

	port := options.MetricsPort
	if port == 0 {
		port = 3000
	}
	server := &http.Server{Addr: fmt.Sprintf(":%v", port), Handler: promhttp.Handler()}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			logger.Errorf("http server failed: %v", err)
		}
	}()
	stress := Stress{
		Name:           t.Name(),
		Nonce:          uuid.Must(uuid.NewUUID()).String()[:8],
		Workers:        NewWorkerPool(options.WorkerTimeout, logger, f),
		Runner:         NewRunner(),
		Schedule:       options.Schedule,
		ReportInterval: options.ReportInterval,
		Logger:         logger,
		MetricsPort:    port,
	}
	return stress, func() {
		logger.Infof("shutdown gostress")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
}

func (s *Stress) RunLocal(ctx context.Context) {
	shutdown := Monitor(s.Name, s.ReportInterval, s.Logger)
	defer shutdown()
	err := s.Runner.RunSchedule(ctx, s.Schedule, s.Workers, s.Logger)
	if err != nil {
		s.Logger.Error("run schedule failed with error: %v", err)
	} else {
		s.Logger.Infof("run schedule finished successfully")
	}
}
