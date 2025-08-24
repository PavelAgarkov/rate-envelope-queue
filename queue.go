package rate_envelope_queue

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

type RateEnvelopeQueue struct {
	ctx context.Context

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

	queueStamps []Stamp // глобальные stamps очереди
}

func chain(base Invoker, stamps ...Stamp) Invoker {
	w := base
	for i := len(stamps) - 1; i >= 0; i-- {
		w = stamps[i](w)
	}
	return w
}

func (rateQueue *RateEnvelopeQueue) buildInvokerChain(e *Envelope) Invoker {
	base := func(ctx context.Context, env *Envelope) error {
		return env.invoke(ctx)
	}
	// порядок: сначала глобальные, потом пер-задачные
	inv := chain(base, append(rateQueue.queueStamps, e.stamps...)...)
	return inv
}

func (rateQueue *RateEnvelopeQueue) worker(ctx context.Context) {
	if rateQueue.waiting {
		defer rateQueue.wg.Done()
	}
	for {
		envelope, shutdown := rateQueue.queue.Get()
		if shutdown {
			log.Printf(service + ": worker is shutting down")
			return
		}

		if rateQueue.checkInBlacklist(envelope._type) {
			rateQueue.queue.Forget(envelope)
			rateQueue.queue.Done(envelope)
			continue
		}

		err := func(envelope *Envelope) error {
			defer func() {
				if r := recover(); r != nil {
					rateQueue.queue.Forget(envelope)
					log.Printf(service+": panic recovered in envelope: %v\n%s", r, debug.Stack())
				}
			}()
			defer rateQueue.queue.Done(envelope)

			tctx := ctx
			var tcancel context.CancelFunc = func() {}
			if envelope.deadline > 0 {
				tctx, tcancel = context.WithTimeout(ctx, envelope.deadline)
			}
			defer tcancel()

			invokerChain := rateQueue.buildInvokerChain(envelope)
			err := invokerChain(tctx, envelope)

			if err == nil && tctx.Err() != nil {
				err = tctx.Err()
			}

			alive := rateQueue.run.Load() && rateQueue.ctx != nil && rateQueue.ctx.Err() == nil

			switch {
			case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
				rateQueue.queue.Forget(envelope)
				if envelope.interval > 0 && alive {
					rateQueue.queue.AddAfter(envelope, envelope.interval)
				}
				return nil
			case errors.Is(err, ErrStopEnvelope):
				rateQueue.queue.Forget(envelope)
				rateQueue.setToBlacklist(envelope._type)
				return nil
			case err != nil:
				if envelope.interval > 0 && alive {
					rateQueue.queue.AddRateLimited(envelope)
				} else {
					rateQueue.queue.Forget(envelope)
				}
				return nil
			default:
				rateQueue.queue.Forget(envelope)
				if envelope.interval > 0 && alive {
					rateQueue.queue.AddAfter(envelope, envelope.interval)
				}
				return nil
			}
		}(envelope)

		if err != nil {
			log.Printf(service+": envelope %s/%d error: %v", envelope._type, envelope.id, err)
		}
	}
}

func NewRateEnvelopeQueue(ctx context.Context, options ...func(*RateEnvelopeQueue)) QueuePool {
	if ctx == nil {
		ctx = context.Background()
	}

	queue := &RateEnvelopeQueue{
		waiting:   true,
		blacklist: make(map[string]struct{}),
		ctx:       ctx,
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
		if envelope._type == "" || envelope.invoke == nil || envelope.interval < 0 || envelope.deadline < 0 {
			return ErrAdditionEnvelopeToQueueBadFields
		}
		if envelope.interval > 0 && envelope.deadline > envelope.interval {
			return ErrAdditionEnvelopeToQueueBadIntervals
		}
		if rateQueue.checkInBlacklist(envelope._type) {
			return ErrEnvelopeInBlacklist
		}
	}

	return nil
}

func (rateQueue *RateEnvelopeQueue) Start() {
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
			rateQueue.worker(rateQueue.ctx)
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
