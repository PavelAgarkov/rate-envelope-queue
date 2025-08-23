package pkg

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
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

	service = "[rate-envelope-queue]"
)

var (
	ErrStopEnvelope                        = errors.New(fmt.Sprintf("%s: stop envelope", service))
	ErrEnvelopeInBlacklist                 = errors.New(fmt.Sprintf("%s: envelope is in blacklist", service))
	ErrEnvelopeQueueIsNotRunning           = errors.New(fmt.Sprintf("%s: queue is not running", service))
	ErrAdditionEnvelopeToQueueBadFields    = errors.New(fmt.Sprintf("%s: addition envelope to queue has bad fields", service))
	ErrAdditionEnvelopeToQueueBadIntervals = errors.New(fmt.Sprintf("%s: addition envelope to queue has bad intervals", service))
)

type Envelope struct {
	Id       uint64
	Type     string
	Interval time.Duration
	Deadline time.Duration

	BeforeHook func(ctx context.Context, item *Envelope) error
	Invoke     func(ctx context.Context) error
	AfterHook  func(ctx context.Context, item *Envelope) error
}

type RateEnvelopeQueue struct {
	limit         int
	queue         workqueue.TypedRateLimitingInterface[*Envelope]
	limiter       workqueue.TypedRateLimiter[*Envelope]
	workqueueConf *workqueue.TypedRateLimitingQueueConfig[*Envelope]

	waiting bool
	wg      sync.WaitGroup

	run      atomic.Bool
	stopMode StopMode

	blackListMu sync.RWMutex
	blacklist   map[string]struct{}
}

func WithLimitOption(limit int) func(*RateEnvelopeQueue) {
	return func(queue *RateEnvelopeQueue) {
		queue.limit = limit
	}
}

func WithWaitingOption(waiting bool) func(*RateEnvelopeQueue) {
	return func(queue *RateEnvelopeQueue) {
		queue.waiting = waiting
	}
}

func WithStopModeOption(stopMode StopMode) func(*RateEnvelopeQueue) {
	return func(queue *RateEnvelopeQueue) {
		if stopMode != Drain && stopMode != Stop {
			panic("invalid stop mode")
		}
		queue.stopMode = stopMode
	}
}

func WithWorkqueueConfigOption(conf *workqueue.TypedRateLimitingQueueConfig[*Envelope]) func(*RateEnvelopeQueue) {
	return func(queue *RateEnvelopeQueue) {
		if conf != nil {
			queue.workqueueConf = conf
		}
	}
}

func WithLimiterOption(limiter workqueue.TypedRateLimiter[*Envelope]) func(*RateEnvelopeQueue) {
	return func(queue *RateEnvelopeQueue) {
		if limiter != nil {
			queue.limiter = limiter
		}
	}
}

func NewRateEnvelopeQueue(options ...func(*RateEnvelopeQueue)) QueuePool {
	queue := &RateEnvelopeQueue{
		waiting:   true,
		blacklist: make(map[string]struct{}),
	}
	for _, option := range options {
		option(queue)
	}

	if queue.limiter == nil {
		queue.limiter = workqueue.NewTypedMaxOfRateLimiter[*Envelope](
			workqueue.NewTypedItemExponentialFailureRateLimiter[*Envelope](1*time.Second, 30*time.Second),
			&workqueue.TypedBucketRateLimiter[*Envelope]{Limiter: rate.NewLimiter(5, 10)},
		)
	}

	if queue.limit <= 0 {
		panic(service + ": limit must be greater than 0")
	}

	if queue.stopMode == "" {
		panic(service + ": stopMode must be set")
	}

	queue.run.Store(true)

	return queue
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

func (rateQueue *RateEnvelopeQueue) worker(ctx context.Context) {
	if rateQueue.waiting {
		defer rateQueue.wg.Done()
	}
	for {
		item, shutdown := rateQueue.queue.Get()
		if shutdown {
			log.Printf(service + ": worker is shutting down")
			return
		}

		if rateQueue.checkInBlacklist(item.Type) {
			rateQueue.queue.Forget(item)
			rateQueue.queue.Done(item)
			continue
		}

		if item.BeforeHook != nil {
			hctx, cancel := withHookTimeout(ctx, item.Deadline, 0.2, 800*time.Millisecond)
			err := item.BeforeHook(hctx, item)
			cancel()

			if err != nil {
				if errors.Is(err, ErrStopEnvelope) {
					rateQueue.setToBlacklist(item.Type)
				}
				if item.Interval > 0 && !errors.Is(err, ErrStopEnvelope) {
					rateQueue.queue.AddRateLimited(item)
				} else {
					rateQueue.queue.Forget(item)
				}
				rateQueue.queue.Done(item)
				log.Printf(service+": envelope %s/%d before hook error: %v", item.Type, item.Id, err)
				continue
			}
		}

		err := func(envelope *Envelope) error {
			defer rateQueue.queue.Done(envelope)

			tctx := ctx
			var tcancel context.CancelFunc = func() {}
			if envelope.Deadline > 0 {
				tctx, tcancel = context.WithTimeout(ctx, envelope.Deadline)
			}
			defer tcancel()

			err := envelope.Invoke(tctx)
			if err == nil && tctx.Err() != nil {
				err = tctx.Err()
			}

			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				rateQueue.queue.Forget(envelope)
				if envelope.Interval > 0 {
					rateQueue.queue.AddAfter(envelope, envelope.Interval)
				}
				return nil
			case errors.Is(err, ErrStopEnvelope):
				rateQueue.queue.Forget(envelope)
				rateQueue.setToBlacklist(envelope.Type)
				return nil
			case err != nil:
				if envelope.Interval > 0 {
					rateQueue.queue.AddRateLimited(envelope)
				} else {
					rateQueue.queue.Forget(envelope)
				}
				return nil
			default:
				rateQueue.queue.Forget(envelope)
				if envelope.Interval > 0 {
					rateQueue.queue.AddAfter(envelope, envelope.Interval)
				}
				return nil
			}
		}(item)

		if err != nil {
			log.Printf(service+": envelope %s/%d error: %v", item.Type, item.Id, err)
		}

		if item.AfterHook != nil {
			hctx, cancel := withHookTimeout(ctx, item.Deadline, 0.2, 800*time.Millisecond)
			err := item.AfterHook(hctx, item)
			cancel()

			if err != nil {
				if errors.Is(err, ErrStopEnvelope) {
					rateQueue.setToBlacklist(item.Type)
				}
				log.Printf(service+": envelope %s/%d after hook error: %v", item.Type, item.Id, err)
			}
		}
	}
}

func (rateQueue *RateEnvelopeQueue) setToBlacklist(types ...string) {
	rateQueue.blackListMu.Lock()
	defer rateQueue.blackListMu.Unlock()
	for _, t := range types {
		rateQueue.blacklist[t] = struct{}{}
	}
}

func (rateQueue *RateEnvelopeQueue) removeFromBlacklist(types ...string) {
	rateQueue.blackListMu.Lock()
	defer rateQueue.blackListMu.Unlock()
	for _, t := range types {
		delete(rateQueue.blacklist, t)
	}
}

func (rateQueue *RateEnvelopeQueue) checkInBlacklist(t string) bool {
	rateQueue.blackListMu.RLock()
	defer rateQueue.blackListMu.RUnlock()
	_, exists := rateQueue.blacklist[t]
	return exists
}

func (rateQueue *RateEnvelopeQueue) Add(envelopes ...*Envelope) error {
	if err := rateQueue.validateAdd(envelopes...); err != nil {
		return err
	}

	for _, envelope := range envelopes {
		rateQueue.queue.Add(envelope)
	}

	return nil
}

func (rateQueue *RateEnvelopeQueue) validateAdd(envelopes ...*Envelope) error {
	if !rateQueue.run.Load() || rateQueue.queue == nil {
		return ErrEnvelopeQueueIsNotRunning
	}

	for _, envelope := range envelopes {
		if envelope.Type == "" || envelope.Invoke == nil || envelope.Interval < 0 || envelope.Deadline < 0 {
			return ErrAdditionEnvelopeToQueueBadFields
		}
		if envelope.Interval > 0 && envelope.Deadline > envelope.Interval {
			return ErrAdditionEnvelopeToQueueBadIntervals
		}
		if rateQueue.checkInBlacklist(envelope.Type) {
			return ErrEnvelopeInBlacklist
		}
	}

	return nil
}

func (rateQueue *RateEnvelopeQueue) Start(ctx context.Context) {
	if !rateQueue.run.Load() || rateQueue.queue != nil {
		log.Printf(service + ": queue is not in a startable state")
		return
	}

	switch rateQueue.workqueueConf {
	case nil:
		rateQueue.queue = workqueue.NewTypedRateLimitingQueue[*Envelope](rateQueue.limiter)
	default:
		rateQueue.queue = workqueue.NewTypedRateLimitingQueueWithConfig[*Envelope](
			rateQueue.limiter,
			*rateQueue.workqueueConf,
		)
	}

	for range rateQueue.limit {
		if rateQueue.waiting {
			rateQueue.wg.Add(1)
		}
		go func() {
			defer recoverWrap()
			rateQueue.worker(ctx)
		}()
	}
}

func (rateQueue *RateEnvelopeQueue) drain() {
	if !rateQueue.run.Load() || rateQueue.queue == nil {
		log.Printf(service + ": queue is already stopped")
		return
	}
	rateQueue.queue.ShutDownWithDrain()
	if rateQueue.waiting {
		rateQueue.wg.Wait()
	}
	rateQueue.run.Store(false)
	log.Printf(service + ": queue is drained")
}

func (rateQueue *RateEnvelopeQueue) stop() {
	if !rateQueue.run.Load() || rateQueue.queue == nil {
		log.Printf(service + " queue is already stopped")
		return
	}
	rateQueue.queue.ShutDown()
	if rateQueue.waiting {
		rateQueue.wg.Wait()
	}
	rateQueue.run.Store(false)
	log.Printf(service + " queue is stopped")
}

func (rateQueue *RateEnvelopeQueue) Stop() {
	switch rateQueue.stopMode {
	case Drain:
		rateQueue.drain()
		return
	case Stop:
		rateQueue.stop()
		return
	}
}

func recoverWrap() {
	if r := recover(); r != nil {
		log.Printf(service+": panic recovered: %v\n%s", r, debug.Stack())
	}
}
