package gostress

import (
	"go.uber.org/zap"
	"sync"
	"time"
)

type (
	StressFn   func(ctx RequestContext) error
	WorkerPool struct {
		F        StressFn
		Lock     sync.Mutex
		Work     chan Id
		Workers  []Worker
		WorkerId Id
		Timeout  time.Duration
		Logger   *zap.SugaredLogger
	}
)

func NewWorkerPool(workerTimeout time.Duration, logger *zap.SugaredLogger, f StressFn) *WorkerPool {
	return &WorkerPool{
		F:       f,
		Work:    make(chan Id),
		Workers: make([]Worker, 0),
		Timeout: workerTimeout,
		Logger:  logger,
	}
}

func (p *WorkerPool) Kill() {
	if len(p.Workers) == 0 {
		return
	}
	last := p.Workers[len(p.Workers)-1]
	last.Shutdown <- struct{}{}
	p.Workers = p.Workers[:len(p.Workers)-1]
}

func (p *WorkerPool) Adjust(size int) {
	if len(p.Workers) == size {
		return
	}
	p.Lock.Lock()
	defer p.Lock.Unlock()
	if len(p.Workers) == size {
		return
	}
	p.Logger.Infof("adjusting workers pool: current=%v, target=%v", len(p.Workers), size)
	for len(p.Workers) < size {
		p.Spawn()
	}
	for len(p.Workers) > size {
		p.Kill()
	}
}

func (p *WorkerPool) Spawn() {
	w := Worker{WorkerId: p.WorkerId, Shutdown: make(chan struct{}, 1)}
	p.WorkerId++
	go func() { w.Run(p.Work, p.Timeout, p.F, p.Logger) }()
	p.Workers = append(p.Workers, w)
}
