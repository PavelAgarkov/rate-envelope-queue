package pkg

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

type StopMode string

const (
	Drain StopMode = "drain"
	Stop  StopMode = "stop"
)

type PoolTask struct {
	Id       int
	Type     string
	Interval time.Duration
	Call     func(ctx context.Context) error
	Deadline time.Duration
}

type QueuePool struct {
	limit         int
	queue         workqueue.TypedRateLimitingInterface[*PoolTask]
	limiter       workqueue.TypedRateLimiter[*PoolTask]
	workqueueConf *workqueue.TypedRateLimitingQueueConfig[*PoolTask]

	waiting bool
	wg      sync.WaitGroup

	run      atomic.Bool
	stopMode StopMode
}

func NewPool(limit int, waiting bool, stopMode StopMode, limiter workqueue.TypedRateLimiter[*PoolTask], workqueueConf *workqueue.TypedRateLimitingQueueConfig[*PoolTask]) Pool {
	if stopMode != Drain && stopMode != Stop {
		panic("invalid stop mode")
	}

	pool := &QueuePool{
		limit:    limit,
		waiting:  waiting,
		stopMode: stopMode,
	}
	if limiter == nil {
		limiter = workqueue.NewTypedMaxOfRateLimiter[*PoolTask](
			workqueue.NewTypedItemExponentialFailureRateLimiter[*PoolTask](1*time.Second, 30*time.Second),
			&workqueue.TypedBucketRateLimiter[*PoolTask]{Limiter: rate.NewLimiter(5, 10)},
		)
	}
	pool.limiter = limiter

	if workqueueConf != nil {
		pool.workqueueConf = workqueueConf
	}
	pool.run.Store(true)

	return pool
}

func (pool *QueuePool) worker(ctx context.Context) {
	if pool.waiting {
		defer pool.wg.Done()
	}
	for {
		item, shutdown := pool.queue.Get()
		if shutdown {
			log.Printf("worker is shutting down")
			return
		}

		func(task *PoolTask) {
			defer pool.queue.Done(task)

			taskCtx, cancel := context.WithTimeout(ctx, task.Deadline)
			defer cancel()

			err := task.Call(taskCtx)
			if err == nil && taskCtx.Err() != nil {
				err = taskCtx.Err()
			}

			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				pool.queue.Forget(task)
				if task.Interval > 0 {
					pool.queue.AddAfter(task, task.Interval)
				}
				return
			case err != nil:
				pool.queue.AddRateLimited(task)
				return
			default:
				pool.queue.Forget(task)
				if task.Interval > 0 {
					pool.queue.AddAfter(task, task.Interval)
				}
				return
			}
		}(item)
	}
}

func (pool *QueuePool) drain() {
	if !pool.run.Load() || pool.queue == nil {
		log.Printf("pool is already stopped")
		return
	}
	pool.queue.ShutDownWithDrain()
	if pool.waiting {
		pool.wg.Wait()
	}
	pool.run.Store(false)
	log.Printf("pool is drained")
}

func (pool *QueuePool) stop() {
	if !pool.run.Load() || pool.queue == nil {
		log.Printf("pool is already stopped")
		return
	}
	pool.queue.ShutDown()
	if pool.waiting {
		pool.wg.Wait()
	}
	pool.run.Store(false)
	log.Printf("pool is stopped")
}

func (pool *QueuePool) Stop() {
	switch pool.stopMode {
	case Drain:
		pool.drain()
		return
	case Stop:
		pool.stop()
		return
	}
}

func (pool *QueuePool) Add(task ...*PoolTask) error {
	if !pool.run.Load() || pool.queue == nil {
		return errors.New("pool is not running")
	}
	for _, t := range task {
		pool.queue.Add(t)
	}

	return nil
}

func (pool *QueuePool) Start(ctx context.Context) {
	if !pool.run.Load() {
		log.Printf("pool is stopped")
		return
	}

	switch pool.workqueueConf {
	case nil:
		pool.queue = workqueue.NewTypedRateLimitingQueue[*PoolTask](pool.limiter)
	default:
		pool.queue = workqueue.NewTypedRateLimitingQueueWithConfig[*PoolTask](
			pool.limiter,
			*pool.workqueueConf,
		)
	}

	for range pool.limit {
		if pool.waiting {
			pool.wg.Add(1)
		}
		go func() {
			defer recoverWrap()
			pool.worker(ctx)
		}()
	}
}

func recoverWrap() {
	if r := recover(); r != nil {
		log.Printf("Recovering from panic: %v", r)
	}
}
