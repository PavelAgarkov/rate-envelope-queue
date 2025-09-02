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

// внутренний автомат состояний
type queueState int32

const (
	stateInit     queueState = iota // создана, ещё не стартовала; Add() — буферизуется
	stateRunning                    // работает; Add() — сразу в workqueue
	stateStopping                   // идёт останов; Add() — ошибка
	stateStopped                    // остановлена; Add() — ошибка; возможен повторный Start()
)

type RateEnvelopeQueue struct {
	ctx context.Context

	limit         int
	queue         workqueue.TypedRateLimitingInterface[*Envelope]
	limiter       workqueue.TypedRateLimiter[*Envelope]
	workqueueConf *workqueue.TypedRateLimitingQueueConfig[*Envelope]

	waiting bool
	wg      sync.WaitGroup

	stopMode StopMode

	run   atomic.Bool // быстрый флаг «жива ли очередь» для воркеров при перепланировании
	state queueState

	// защита старт/стоп/смена очереди/смена состояния
	lifecycleMu sync.Mutex
	// защита только чтения состояния
	stateMu sync.RWMutex

	queueStamps []Stamp // глобальные stamps очереди

	// Буфер задач, добавленных до первого Start()
	pendingMu sync.Mutex
	pending   []*Envelope
}

func chain(base Invoker, stamps ...Stamp) Invoker {
	w := base
	for i := len(stamps) - 1; i >= 0; i-- {
		w = stamps[i](w)
	}
	return w
}

func (q *RateEnvelopeQueue) buildInvokerChain(e *Envelope) Invoker {
	base := func(ctx context.Context, env *Envelope) error { return env.invoke(ctx) }
	return chain(base, append(q.queueStamps, e.stamps...)...)
}

func (q *RateEnvelopeQueue) currentState() queueState {
	q.stateMu.RLock()
	s := q.state
	q.stateMu.RUnlock()
	return s
}

func (q *RateEnvelopeQueue) setState(s queueState) {
	q.stateMu.Lock()
	q.state = s
	q.stateMu.Unlock()
}

func (q *RateEnvelopeQueue) worker(ctx context.Context) {
	if q.waiting {
		defer q.wg.Done()
	}
	for {
		envelope, shutdown := q.queue.Get()
		if shutdown {
			log.Printf(service + ": worker is shutting down")
			return
		}

		err := func(envelope *Envelope) error {
			defer q.queue.Done(envelope)
			defer func() {
				if r := recover(); r != nil {
					// важен порядок: Forget до выхода (Done отработает после этого defer)
					q.queue.Forget(envelope)
					log.Printf(service+": panic recovered in envelope: %v\n%s", r, debug.Stack())
				}
			}()

			tctx := ctx
			var tcancel context.CancelFunc = func() {}
			if envelope.deadline > 0 {
				tctx, tcancel = context.WithTimeout(ctx, envelope.deadline)
			}
			defer tcancel()

			var err error
			if envelope.beforeHook != nil {
				hctx, cancel := WithHookTimeout(ctx, envelope.deadline, 0.5, 800*time.Millisecond)
				err = envelope.beforeHook(hctx, envelope)
				cancel()
			}

			invoker := q.buildInvokerChain(envelope)
			err = invoker(tctx, envelope)

			if envelope.afterHook != nil {
				hctx, cancel := WithHookTimeout(ctx, envelope.deadline, 0.5, 800*time.Millisecond)
				err = envelope.afterHook(hctx, envelope)
				cancel()
			}

			if err == nil && tctx.Err() != nil {
				err = tctx.Err()
			}

			alive := q.run.Load() && q.ctx != nil && q.ctx.Err() == nil && q.currentState() == stateRunning

			switch {
			case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
				q.queue.Forget(envelope)
				if envelope.interval > 0 && alive {
					q.queue.AddAfter(envelope, envelope.interval)
				}
				return nil
			case errors.Is(err, ErrStopEnvelope):
				q.queue.Forget(envelope)
				return nil
			case err != nil:
				if envelope.interval > 0 && alive {
					q.queue.AddRateLimited(envelope)
				} else {
					q.queue.Forget(envelope)
				}
				return nil
			default:
				q.queue.Forget(envelope)
				if envelope.interval > 0 && alive {
					q.queue.AddAfter(envelope, envelope.interval)
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
	q := &RateEnvelopeQueue{
		ctx:     ctx,
		waiting: true,
		state:   stateInit,
	}
	for _, o := range options {
		o(q)
	}

	if q.limiter == nil {
		q.limiter = workqueue.NewTypedMaxOfRateLimiter[*Envelope](
			workqueue.NewTypedItemExponentialFailureRateLimiter[*Envelope](1*time.Second, 30*time.Second),
			&workqueue.TypedBucketRateLimiter[*Envelope]{Limiter: rate.NewLimiter(5, 10)},
		)
	}
	if q.limit <= 0 {
		panic(service + ": limit must be greater than 0")
	}
	if q.stopMode == "" {
		panic(service + ": stopMode must be set")
	}
	// в init очередь ещё не «живая»
	q.run.Store(false)

	return q
}

func (q *RateEnvelopeQueue) Add(envelopes ...*Envelope) error {
	// валидация содержимого (не состояния)
	for _, e := range envelopes {
		if e == nil {
			return ErrAdditionEnvelopeToQueueBadFields
		}
		if e._type == "" || e.invoke == nil || e.interval < 0 || e.deadline < 0 {
			return ErrAdditionEnvelopeToQueueBadFields
		}
		if e.interval > 0 && e.deadline > e.interval {
			return ErrAdditionEnvelopeToQueueBadIntervals
		}
	}

	switch q.currentState() {
	case stateInit:
		q.pendingMu.Lock()
		q.pending = append(q.pending, envelopes...)
		q.pendingMu.Unlock()
		return nil
	case stateRunning:
		for _, e := range envelopes {
			q.queue.Add(e)
		}
		return nil
	default:
		return ErrEnvelopeQueueIsNotRunning
	}
}

func (q *RateEnvelopeQueue) Start() {
	q.lifecycleMu.Lock()
	defer q.lifecycleMu.Unlock()

	switch q.state {
	case stateRunning:
		return
	case stateStopping:
		log.Printf(service + ": queue is stopping; Start skipped")
		return
	}

	// recreate workqueue
	if q.workqueueConf == nil {
		q.queue = workqueue.NewTypedRateLimitingQueue[*Envelope](q.limiter)
	} else {
		q.queue = workqueue.NewTypedRateLimitingQueueWithConfig[*Envelope](
			q.limiter,
			*q.workqueueConf,
		)
	}

	// переключаем состояние и run-флаг
	q.setState(stateRunning)
	q.run.Store(true)

	// запустить воркеры
	for i := 0; i < q.limit; i++ {
		if q.waiting {
			q.wg.Add(1)
		}
		go func() {
			defer recoverWrap()
			q.worker(q.ctx)
		}()
	}

	// слить pending
	q.pendingMu.Lock()
	if len(q.pending) > 0 {
		for _, e := range q.pending {
			q.queue.Add(e)
		}
		q.pending = nil
	}
	q.pendingMu.Unlock()
}

func (q *RateEnvelopeQueue) Stop() {
	q.lifecycleMu.Lock()
	if q.state != stateRunning {
		q.lifecycleMu.Unlock()
		return
	}
	q.setState(stateStopping)
	q.run.Store(false) // запрещаем перепланирование
	q.lifecycleMu.Unlock()

	switch q.stopMode {
	case Drain:
		if q.queue != nil {
			q.queue.ShutDownWithDrain()
		}
	default:
		if q.queue != nil {
			q.queue.ShutDown()
		}
	}

	if q.waiting {
		q.wg.Wait()
	}

	q.lifecycleMu.Lock()
	q.setState(stateStopped)
	q.queue = nil
	q.lifecycleMu.Unlock()

	log.Printf(service + ": queue is drained/stopped")
}

func recoverWrap() {
	if r := recover(); r != nil {
		log.Printf(service+": panic recovered: %v\n%s", r, debug.Stack())
	}
}

func WithLimitOption(limit int) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) { q.limit = limit }
}

func WithWaitingOption(waiting bool) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) { q.waiting = waiting }
}

func WithStopModeOption(mode StopMode) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		if mode != Drain && mode != Stop {
			panic("invalid stop mode")
		}
		q.stopMode = mode
	}
}

func WithWorkqueueConfigOption(conf *workqueue.TypedRateLimitingQueueConfig[*Envelope]) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		if conf != nil {
			q.workqueueConf = conf
		}
	}
}

func WithLimiterOption(limiter workqueue.TypedRateLimiter[*Envelope]) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		if limiter != nil {
			q.limiter = limiter
		}
	}
}
