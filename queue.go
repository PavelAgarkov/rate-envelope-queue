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
	queueMu       sync.RWMutex
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
	base := func(ctx context.Context, env *Envelope) error {
		return env.invoke(ctx, env)
	}
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
	q.queueMu.RLock()
	queue := q.queue
	q.queueMu.RUnlock()
	if queue == nil {
		log.Printf(service + ": worker: queue is nil; exiting")
		return
	}

	for {
		envelope, shutdown := queue.Get()
		if shutdown {
			log.Printf(service + ": worker is shutting down")
			return
		}

		err := func(envelope *Envelope) error {
			defer queue.Done(envelope)
			defer func() {
				if r := recover(); r != nil {
					// важен порядок: Forget до выхода (Done отработает после этого defer)
					queue.Forget(envelope)
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
			// отмена/таймаут — забыть и, если периодическая и очередь жива, перепланировать
			case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
				queue.Forget(envelope)
				if envelope.interval > 0 && alive {
					queue.AddAfter(envelope, envelope.interval)
				}
				return nil

			// ErrStopEnvelope — забыть и не перепланировать. Ошибка от пользователя о том, что задача больше не нужна
			case errors.Is(err, ErrStopEnvelope):
				queue.Forget(envelope)
				return nil

			// любая другая ошибка — перепланировать (если периодическая и очередь жива) и, если одиночная, вызвать failureHook (если есть) и реагировать
			//на ответ пользователя через DestinationResult
			case err != nil:
				if envelope.interval > 0 && alive {
					queue.AddAfter(envelope, envelope.interval)
				}

				// одиночная задача с ошибкой — срабатывает failureHook (если есть) и забывается
				if envelope.interval == 0 && envelope.failureHook != nil {
					// на этот хук даем общее время дедлайна, важная часть. Нужно дать возомжность отправить
					//ошибку в сторонний сервис. Снаружи пользователь управляет временем через deadline
					decision := envelope.failureHook(tctx, envelope, err)

					if decision == nil {
						decision = NewDefaultDestination()
					}

					payload := decision.Payload()
					t, ok := payload[DestinationStateField]
					if !ok {
						payload[DestinationStateField] = DestinationStateDrop
					}

					if alive {
						switch t {
						case DestinationStateRetryNow:
							queue.Add(envelope)
						case DestinationStataRetryAfter:
							delay, ok := payload[PayloadAfterField]
							if !ok {
								log.Printf(service + ": envelope failureHook did not return after field; using 30s, please customize field")
								delay = 30 * time.Second
							}

							delayInterval, assert := delay.(time.Duration)
							if !assert {
								log.Printf(service + ": envelope failureHook returned invalid after field; using 30s, please customize field")
								delayInterval = 30 * time.Second
							}

							if delayInterval > 0 {
								queue.AddAfter(envelope, delayInterval)
							}
						case DestinationStateDrop:
						}
					}

					queue.Forget(envelope)

				}
				return nil

			default:
				queue.Forget(envelope)
				if envelope.interval > 0 && alive {
					queue.AddAfter(envelope, envelope.interval)
				}
				if envelope.successHook != nil {
					hctx, cancel := WithHookTimeout(ctx, envelope.deadline, 0.5, 800*time.Millisecond)
					envelope.successHook(hctx, envelope)
					cancel()
				}
				return nil
			}
		}(envelope)

		if err != nil {
			log.Printf(service+": envelope %s/%d error: %v", envelope._type, envelope.id, err)
		}
	}
}

func NewRateEnvelopeQueue(base context.Context, options ...func(*RateEnvelopeQueue)) QueuePool {
	q := &RateEnvelopeQueue{
		ctx:     base,
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
		q.queueMu.RLock()
		local := q.queue
		q.queueMu.RUnlock()
		if local == nil {
			return ErrEnvelopeQueueIsNotRunning
		}
		for _, e := range envelopes {
			local.Add(e)
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
	var newQ workqueue.TypedRateLimitingInterface[*Envelope]
	if q.workqueueConf == nil {
		newQ = workqueue.NewTypedRateLimitingQueue[*Envelope](q.limiter)
	} else {
		newQ = workqueue.NewTypedRateLimitingQueueWithConfig[*Envelope](q.limiter, *q.workqueueConf)
	}

	q.queueMu.Lock()
	q.queue = newQ
	q.queueMu.Unlock()

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
		q.queueMu.RLock()
		local := q.queue
		q.queueMu.RUnlock()
		for _, e := range q.pending {
			local.Add(e)
		}
		q.pending = nil
	}
	q.pendingMu.Unlock()
}

func (q *RateEnvelopeQueue) Stop() {
	// Переводим состояние
	q.lifecycleMu.Lock()
	if q.state != stateRunning {
		q.lifecycleMu.Unlock()
		return
	}
	q.setState(stateStopping)
	q.run.Store(false)
	q.lifecycleMu.Unlock()

	// Берём локальную ссылку, НЕ обнуляя публикацию пока идёт drain
	q.queueMu.RLock()
	local := q.queue
	q.queueMu.RUnlock()

	// Останавливаем старую очередь
	if local != nil {
		switch q.stopMode {
		case Drain:
			local.ShutDownWithDrain()
		default:
			local.ShutDown()
		}
	}

	if q.waiting {
		q.wg.Wait()
	}

	// Теперь можно финализировать состояние и обнулить публикуемую ссылку
	q.lifecycleMu.Lock()
	q.setState(stateStopped)
	q.lifecycleMu.Unlock()

	q.queueMu.Lock()
	q.queue = nil
	q.queueMu.Unlock()

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
