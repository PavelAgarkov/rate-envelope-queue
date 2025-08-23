package pkg

import (
	"context"
	"errors"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

var (
	ErrStopTask                       = errors.New("[queue-pool]: stop task")
	ErrTaskInBlacklist                = errors.New("[queue-pool]: task is in blacklist")
	ErrQueuePoolIsNotRunning          = errors.New("[queue-pool]: pool is not running")
	ErrAdditionTaskToPoolBadFields    = errors.New("[queue-pool]: addition task to pool has bad fields")
	ErrAdditionTaskToPoolBadIntervals = errors.New("[queue-pool]: addition task to pool has bad intervals")
)

type StopMode string

const (
	Drain StopMode = "drain"
	Stop  StopMode = "stop"
)

type PoolTask struct {
	Id         uint64
	Type       string
	Interval   time.Duration
	Deadline   time.Duration
	BeforeHook func(ctx context.Context, item *PoolTask) error
	Call       func(ctx context.Context) error
	AfterHook  func(ctx context.Context, item *PoolTask) error
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

	blackListMu sync.RWMutex
	blacklist   map[string]struct{}
}

func WithLimitOption(limit int) func(*QueuePool) {
	return func(pool *QueuePool) {
		pool.limit = limit
	}
}

func WithWaitingOption(waiting bool) func(*QueuePool) {
	return func(pool *QueuePool) {
		pool.waiting = waiting
	}
}

func WithStopModeOption(stopMode StopMode) func(*QueuePool) {
	return func(pool *QueuePool) {
		if stopMode != Drain && stopMode != Stop {
			panic("invalid stop mode")
		}
		pool.stopMode = stopMode
	}
}

func WithWorkqueueConfigOption(conf *workqueue.TypedRateLimitingQueueConfig[*PoolTask]) func(*QueuePool) {
	return func(pool *QueuePool) {
		if conf != nil {
			pool.workqueueConf = conf
		}
	}
}

func WithLimiterOption(limiter workqueue.TypedRateLimiter[*PoolTask]) func(*QueuePool) {
	return func(pool *QueuePool) {
		if limiter == nil {
			pool.limiter = workqueue.NewTypedMaxOfRateLimiter[*PoolTask](
				workqueue.NewTypedItemExponentialFailureRateLimiter[*PoolTask](1*time.Second, 30*time.Second),
				&workqueue.TypedBucketRateLimiter[*PoolTask]{Limiter: rate.NewLimiter(5, 10)},
			)
		} else {
			pool.limiter = limiter
		}
	}
}

func NewPool(options ...func(*QueuePool)) Pool {
	pool := &QueuePool{
		waiting:   true,
		blacklist: make(map[string]struct{}),
	}
	for _, option := range options {
		option(pool)
	}

	if pool.limit <= 0 {
		panic("[queue-pool] limit must be greater than 0")
	}
	if pool.limiter == nil {
		panic("[queue-pool] limiter must be set")
	}

	if pool.stopMode == "" {
		panic("[queue-pool] stopMode must be set")
	}

	pool.run.Store(true)

	return pool
}

func withHookTimeout(ctx context.Context, base time.Duration, frac float64, min time.Duration) (context.Context, context.CancelFunc) {
	d := time.Duration(float64(base) * frac)
	if d < min {
		d = min
	}
	if base == 0 {
		d = min
	}
	return context.WithTimeout(ctx, d)
}

func (pool *QueuePool) worker(ctx context.Context) {
	if pool.waiting {
		defer pool.wg.Done()
	}
	for {
		item, shutdown := pool.queue.Get()
		if shutdown {
			log.Printf("[queue-pool] worker is shutting down")
			return
		}

		if pool.checkInBlacklist(item.Type) {
			pool.queue.Forget(item)
			pool.queue.Done(item)
			continue
		}

		if item.BeforeHook != nil {
			hctx, cancel := withHookTimeout(ctx, item.Deadline, 0.2, 800*time.Millisecond)
			err := item.BeforeHook(hctx, item)
			cancel()

			if err != nil {
				if errors.Is(err, ErrStopTask) {
					pool.setToBlacklist(item.Type)
				}
				if item.Interval > 0 && !errors.Is(err, ErrStopTask) {
					pool.queue.AddRateLimited(item)
				} else {
					pool.queue.Forget(item)
				}
				pool.queue.Done(item)
				log.Printf("[queue-pool] task %s/%d before hook error: %v", item.Type, item.Id, err)
				continue
			}
		}

		err := func(task *PoolTask) error {
			defer pool.queue.Done(task)

			tctx := ctx
			var tcancel context.CancelFunc = func() {}
			if task.Deadline > 0 {
				tctx, tcancel = context.WithTimeout(ctx, task.Deadline)
			}
			defer tcancel()

			err := task.Call(tctx)
			if err == nil && tctx.Err() != nil {
				err = tctx.Err()
			}

			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				pool.queue.Forget(task)
				if task.Interval > 0 {
					pool.queue.AddAfter(task, task.Interval)
				}
				return nil
			case errors.Is(err, ErrStopTask):
				pool.queue.Forget(task)
				pool.setToBlacklist(task.Type)
				return nil
			case err != nil:
				if task.Interval > 0 {
					pool.queue.AddRateLimited(task)
				} else {
					pool.queue.Forget(task)
				}
				return nil
			default:
				pool.queue.Forget(task)
				if task.Interval > 0 {
					pool.queue.AddAfter(task, task.Interval)
				}
				return nil
			}
		}(item)

		if err != nil {
			log.Printf("[queue-pool] task %s/%d error: %v", item.Type, item.Id, err)
		}

		if item.AfterHook != nil {
			hctx, cancel := withHookTimeout(ctx, item.Deadline, 0.2, 800*time.Millisecond)
			err := item.AfterHook(hctx, item)
			cancel()

			if err != nil {
				if errors.Is(err, ErrStopTask) {
					pool.setToBlacklist(item.Type)
				}
				log.Printf("[queue-pool] task %s/%d after hook error: %v", item.Type, item.Id, err)
			}
		}
	}
}

func (pool *QueuePool) drain() {
	if !pool.run.Load() || pool.queue == nil {
		log.Printf("[queue-pool] pool is already stopped")
		return
	}
	pool.queue.ShutDownWithDrain()
	if pool.waiting {
		pool.wg.Wait()
	}
	pool.run.Store(false)
	log.Printf("[queue-pool] pool is drained")
}

func (pool *QueuePool) stop() {
	if !pool.run.Load() || pool.queue == nil {
		log.Printf("[queue-pool] pool is already stopped")
		return
	}
	pool.queue.ShutDown()
	if pool.waiting {
		pool.wg.Wait()
	}
	pool.run.Store(false)
	log.Printf("[queue-pool] pool is stopped")
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

func (pool *QueuePool) setToBlacklist(types ...string) {
	pool.blackListMu.Lock()
	defer pool.blackListMu.Unlock()
	for _, t := range types {
		pool.blacklist[t] = struct{}{}
	}
}

func (pool *QueuePool) removeFromBlacklist(types ...string) {
	pool.blackListMu.Lock()
	defer pool.blackListMu.Unlock()
	for _, t := range types {
		delete(pool.blacklist, t)
	}
}

func (pool *QueuePool) checkInBlacklist(t string) bool {
	pool.blackListMu.RLock()
	defer pool.blackListMu.RUnlock()
	_, exists := pool.blacklist[t]
	return exists
}

func (pool *QueuePool) Add(task ...*PoolTask) error {
	if err := pool.validateAdd(task...); err != nil {
		return err
	}

	for _, t := range task {
		pool.queue.Add(t)
	}

	return nil
}

func (pool *QueuePool) validateAdd(task ...*PoolTask) error {
	if !pool.run.Load() || pool.queue == nil {
		return ErrQueuePoolIsNotRunning
	}

	for _, t := range task {
		if t.Type == "" || t.Call == nil || t.Interval < 0 || t.Deadline < 0 {
			return ErrAdditionTaskToPoolBadFields
		}
		if t.Interval > 0 && t.Deadline > t.Interval {
			return ErrAdditionTaskToPoolBadIntervals
		}
		if pool.checkInBlacklist(t.Type) {
			return ErrTaskInBlacklist
		}
	}

	return nil
}

func (pool *QueuePool) Start(ctx context.Context) {
	if !pool.run.Load() {
		log.Printf("[queue-pool] pool is stopped")
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
		log.Printf("[queue-pool] panic recovered: %v\n%s", r, debug.Stack())
	}
}
