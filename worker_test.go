package gostress

import (
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSimple(t *testing.T) {
	w := NewWorker(0)
	called := int32(0)
	work := make(chan Id)
	go w.Run(work, 1*time.Second, zaptest.NewLogger(t).Sugar(), func(ctx RequestContext) error {
		atomic.AddInt32(&called, 1)
		return nil
	})
	for i := 0; i < 10; i++ {
		work <- Id(i)
	}
	w.Shutdown <- struct{}{}
	<-w.Finished
	assert.Equal(t, atomic.LoadInt32(&called), int32(10))
}

func TestTimeout(t *testing.T) {
	w := NewWorker(0)
	called := int32(0)
	work := make(chan Id)
	go w.Run(work, 1*time.Second, zaptest.NewLogger(t).Sugar(), func(ctx RequestContext) error {
		atomic.AddInt32(&called, 1)
		if ctx.Id == 0 {
			time.Sleep(1 * time.Minute)
		}
		return nil
	})
	for i := 0; i < 10; i++ {
		work <- Id(i)
	}
	w.Shutdown <- struct{}{}
	<-w.Finished
	assert.Equal(t, atomic.LoadInt32(&called), int32(10))
}
