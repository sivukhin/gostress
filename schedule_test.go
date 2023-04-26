package gostress

import (
	"context"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestName(t *testing.T) {
	registerMetrics("test")
	r := NewRunner()
	l1 := LoadParams{Rps: 500, Workers: 10, Duration: 10 * time.Second}
	requests := int64(0)
	logger := zaptest.NewLogger(t).Sugar()
	pool := NewWorkerPool(time.Second, logger, func(ctx RequestContext) error {
		atomic.AddInt64(&requests, 1)
		return nil
	})
	r.RunSimpleSchedule(context.Background(), l1, l1, pool, logger)
	assert.Greater(t, requests, int64(4900))
	assert.Less(t, requests, int64(5100))
}
