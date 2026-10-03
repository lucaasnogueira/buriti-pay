package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrQueueFull = errors.New("worker queue is full (backpressure)")
)

type Job struct {
	PaymentID uuid.UUID
}

type JobHandler interface {
	Process(ctx context.Context, job Job) error
}

type Pool struct {
	logger      *slog.Logger
	handler     JobHandler
	workerCount int
	jobTimeout  time.Duration
	jobs        chan Job
	wg          sync.WaitGroup
	quit        chan struct{}
	stopOnce    sync.Once
}

func NewPool(logger *slog.Logger, handler JobHandler, workerCount int, bufferSize int, jobTimeout time.Duration) *Pool {
	if workerCount <= 0 {
		workerCount = 4
	}
	if bufferSize <= 0 {
		bufferSize = 100
	}
	if jobTimeout <= 0 {
		jobTimeout = 5 * time.Second
	}

	return &Pool{
		logger:      logger,
		handler:     handler,
		workerCount: workerCount,
		jobTimeout:  jobTimeout,
		jobs:        make(chan Job, bufferSize),
		quit:        make(chan struct{}),
	}
}

// Start spawns the configured N worker goroutines.
func (p *Pool) Start(ctx context.Context) {
	for i := 1; i <= p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
	p.logger.Info("Worker pool started", "workers", p.workerCount, "capacity", cap(p.jobs))
}

func (p *Pool) worker(parentCtx context.Context, id int) {
	defer p.wg.Done()

	for {
		select {
		case <-parentCtx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				// Channel closed and drained
				return
			}
			p.processJobWithRecovery(parentCtx, id, job)
		}
	}
}

func (p *Pool) processJobWithRecovery(parentCtx context.Context, workerID int, job Job) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Worker recovered from panic",
				"worker_id", workerID,
				"payment_id", job.PaymentID.String(),
				"panic", fmt.Sprintf("%v", r),
				"stack", string(debug.Stack()),
			)
		}
	}()

	ctx, cancel := context.WithTimeout(parentCtx, p.jobTimeout)
	defer cancel()

	if err := p.handler.Process(ctx, job); err != nil {
		p.logger.Error("Job processing failed",
			"worker_id", workerID,
			"payment_id", job.PaymentID.String(),
			"error", err,
		)
	}
}

// Enqueue puts a job in the channel or returns ErrQueueFull if saturated.
func (p *Pool) Enqueue(job Job) error {
	select {
	case p.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Stop gracefully shuts down the worker pool, draining remaining in-flight jobs.
func (p *Pool) Stop(timeout time.Duration) error {
	var err error
	p.stopOnce.Do(func() {
		close(p.quit)
		close(p.jobs)

		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			p.logger.Info("Worker pool stopped gracefully")
		case <-time.After(timeout):
			err = errors.New("timed out waiting for worker pool to drain")
			p.logger.Warn("Worker pool shutdown timed out", "timeout", timeout)
		}
	})
	return err
}
