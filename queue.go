package rate_envelope_queue

import (
	"context"
	"errors"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/client-go/util/workqueue"
	_ "k8s.io/component-base/metrics/prometheus/workqueue"
)

// внутренний автомат состояний
type queueState int32

const (
	stateInit     queueState = iota // создана, ещё не стартовала; Add() — буферизуется
	stateRunning                    // работает; Add() — сразу в workqueue
	stateStopping                   // идёт останов; Add() — ошибка
	stateStopped                    // остановлена; Add() — ошибка; возможен повторный Start()
)

const (
	hardHookLimit = 800 * time.Millisecond
	frac          = 0.5
)

type RateEnvelopeQueue struct {
	name string
	ctx  context.Context

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

	allowedCapacity uint64
	currentCapacity atomic.Uint64
}

// NewRateEnvelopeQueue по умолчанию workqueue теряет задачи в режиме AddAfter при любой остановке очереди.
// ----------------------------------------------------------------------------------
// аккуратный режим остановки, прочитаем все что в очереди
// WithWaitingOption(true),
// WithStopModeOption(Drain),
// ----------------------------------------------------------------------------------
// корректно, если нужен «почти drain», но без жёсткого ожидания всех воркеров
// WithWaitingOption(false),
// WithStopModeOption(Drain),
// ----------------------------------------------------------------------------------
// корректно для «быстрого, но чистого» останова с ожиданием
// WithWaitingOption(true),
// WithStopModeOption(Stop),
// ----------------------------------------------------------------------------------
// мгновернный останов без ожидания, все теряем
// WithWaitingOption(false),
// WithStopModeOption(Stop),
// ----------------------------------------------------------------------------------
func NewRateEnvelopeQueue(base context.Context, name string, options ...func(*RateEnvelopeQueue)) SingleQueuePool {
	q := &RateEnvelopeQueue{
		ctx:     base,
		waiting: true,
		state:   stateInit,
		name:    name,
	}
	for _, o := range options {
		o(q)
	}

	if q.limiter == nil {
		q.limiter = workqueue.NewTypedMaxOfRateLimiter[*Envelope]()
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

func NewSimpleDrainQueue(base context.Context, queueName string, queueRate int) SingleQueuePool {
	q := NewRateEnvelopeQueue(
		base,
		queueName,
		WithLimitOption(queueRate),
		WithWaitingOption(true),
		WithStopModeOption(Drain),
		WithAllowedCapacityOption(1_000_000),
	)
	return q
}

func NewSimpleStopQueue(base context.Context, queueName string, queueRate int) SingleQueuePool {
	q := NewRateEnvelopeQueue(
		base,
		queueName,
		WithLimitOption(queueRate),
		WithWaitingOption(false),
		WithStopModeOption(Stop),
		WithAllowedCapacityOption(1_000_000),
	)
	return q
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
			defer q.dec()
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

			var important error = nil
			if envelope.beforeHook != nil {
				hctx, cancel := withHookTimeout(ctx, envelope.deadline, frac, hardHookLimit)
				important = envelope.beforeHook(hctx, envelope)
				cancel()
				if important != nil {
					if errors.Is(important, ErrStopEnvelope) && envelope.afterHook != nil {
						hctx, cancel := withHookTimeout(ctx, envelope.deadline, frac, hardHookLimit)
						_ = envelope.afterHook(hctx, envelope) // ignore result
						cancel()
					}
					goto handle
				}
			}

			{
				invoker := q.buildInvokerChain(envelope)
				important = invoker(tctx, envelope)
				if important != nil {
					if errors.Is(important, ErrStopEnvelope) && envelope.afterHook != nil {
						hctx, cancel := withHookTimeout(ctx, envelope.deadline, frac, hardHookLimit)
						_ = envelope.afterHook(hctx, envelope) // ignore result
						cancel()
					}
					goto handle
				}
			}

			if envelope.afterHook != nil {
				hctx, cancel := withHookTimeout(ctx, envelope.deadline, frac, hardHookLimit)
				important = envelope.afterHook(hctx, envelope)
				cancel()
				if important != nil && !errors.Is(important, ErrStopEnvelope) {
					important = nil // игнорим всё, кроме стопа
				}
				goto handle
			}

		handle:
			if important == nil && tctx.Err() != nil {
				important = tctx.Err()
			}

			alive := q.run.Load() && q.ctx != nil && q.ctx.Err() == nil &&
				q.currentState() == stateRunning && !queue.ShuttingDown()

			switch {
			// отмена/таймаут — забыть и, если периодическая и очередь жива, перепланировать
			case errors.Is(important, context.Canceled) || errors.Is(important, context.DeadlineExceeded):
				queue.Forget(envelope)

				if envelope.interval > 0 && alive {
					queue.AddAfter(envelope, envelope.interval)
					q.inc(1)
				}
				return nil

			// ErrStopEnvelope — забыть и не перепланировать. Ошибка от пользователя о том, что задача больше не нужна
			case errors.Is(important, ErrStopEnvelope):
				queue.Forget(envelope)

				return nil

			// любая другая ошибка — перепланировать (если периодическая и очередь жива) и, если одиночная, вызвать failureHook (если есть) и реагировать
			//на ответ пользователя через DestinationResult
			case important != nil:
				queue.Forget(envelope)

				if envelope.interval > 0 && alive {
					queue.AddAfter(envelope, envelope.interval)
					q.inc(1)
				}

				// одиночная задача с ошибкой — срабатывает failureHook (если есть) и забывается
				if envelope.interval == 0 && envelope.failureHook != nil {
					// на этот хук даем общее время дедлайна, важная часть. Нужно дать возомжность отправить
					//ошибку в сторонний сервис. Снаружи пользователь управляет временем через deadline
					hctx, cancel := withHookTimeout(ctx, envelope.deadline, frac, hardHookLimit)
					decision := envelope.failureHook(hctx, envelope, important)
					cancel()

					if decision == nil {
						decision = DefaultOnceDecision()
					}

					payload := decision.Payload()

					state, ok := payload[DestinationStateField].(destinationState)
					if !ok {
						payload[DestinationStateField] = DecisionStateDrop
						state = DecisionStateDrop
					}

					if alive {
						switch state {
						case DecisionStateRetryNow:
							queue.Add(envelope)
							q.inc(1)
						case DecisionStateRetryAfter:
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
								q.inc(1)
							}
						case DecisionStateDrop:
						}
					}
				}
				return nil

			default:
				queue.Forget(envelope)
				if envelope.interval > 0 && alive {
					queue.AddAfter(envelope, envelope.interval)
					q.inc(1)
				}
				if envelope.successHook != nil {
					hctx, cancel := withHookTimeout(ctx, envelope.deadline, frac, hardHookLimit)
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

func (q *RateEnvelopeQueue) Send(envelopes ...*Envelope) error {
	// валидация содержимого (не состояния)
	for _, e := range envelopes {
		if e == nil {
			return ErrAdditionEnvelopeToQueueBadFields
		}
		if e.invoke == nil || e.interval < 0 || e.deadline < 0 {
			return ErrAdditionEnvelopeToQueueBadFields
		}
		if e.interval > 0 && e.deadline > e.interval {
			return ErrAdditionEnvelopeToQueueBadIntervals
		}
	}

	need := uint64(len(envelopes))

	for {
		s := q.currentState()
		switch s {
		case stateInit, stateStopped:
			q.pendingMu.Lock()
			// повторная проверка состояния под локом
			if q.currentState() == stateInit || q.currentState() == stateStopped {
				// в init/stopped — буферизуем, если есть место
				// (в stopped — на случай, если очередь остановлена и потом снова запущена)
				// в stopped буфер не чистим, т.к. может быть повторный старт
				// проверяем вместимость
				if !q.tryReserve(need) {
					q.pendingMu.Unlock()
					return ErrAllowedQueueCapacityExceeded
				}
				q.pending = append(q.pending, envelopes...)
				q.pendingMu.Unlock()
				return nil
			}
			q.pendingMu.Unlock()
			// состояние сменилось — пробуем снова по новому пути
			continue

		case stateRunning:
			if !q.tryReserve(need) {
				return ErrAllowedQueueCapacityExceeded
			}

			q.lifecycleMu.Lock()
			// повторная проверка под той же блокировкой, что и Stop/Start
			if q.currentState() != stateRunning {
				q.lifecycleMu.Unlock()
				q.unreserve(need)
				return ErrEnvelopeQueueIsNotRunning
			}

			q.queueMu.RLock()
			local := q.queue
			shutting := local == nil || local.ShuttingDown()
			q.queueMu.RUnlock()
			if shutting {
				q.lifecycleMu.Unlock()
				q.unreserve(need)
				return ErrEnvelopeQueueIsNotRunning
			}

			for _, e := range envelopes {
				local.Add(e)
			}
			q.lifecycleMu.Unlock()
			return nil

		default:
			return ErrEnvelopeQueueIsNotRunning
		}
	}
}

func (q *RateEnvelopeQueue) Start() {
	q.lifecycleMu.Lock()
	defer q.lifecycleMu.Unlock()

	switch q.currentState() {
	case stateRunning:
		return
	case stateStopping:
		log.Printf(service + ": queue is stopping; Start skipped")
		return
	}

	// recreate workqueue
	var newQ workqueue.TypedRateLimitingInterface[*Envelope]
	if q.workqueueConf == nil {
		newQ = workqueue.NewTypedRateLimitingQueueWithConfig[*Envelope](q.limiter, workqueue.TypedRateLimitingQueueConfig[*Envelope]{Name: q.name})
	} else {
		q.workqueueConf.Name = q.name
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
	// Переводим состояние в stopping (под "зонтиком"), без долгих операций под локом.
	q.lifecycleMu.Lock()
	if q.currentState() != stateRunning {
		q.lifecycleMu.Unlock()
		return
	}
	q.setState(stateStopping)
	q.run.Store(false)
	q.lifecycleMu.Unlock()

	// Снимок ссылки на очередь (не обнуляем публикацию до финализации).
	q.queueMu.RLock()
	local := q.queue
	q.queueMu.RUnlock()

	// Сигнал остановки очереди.
	if local != nil {
		switch q.stopMode {
		case Drain:
			local.ShutDownWithDrain()
		default: // Stop
			local.ShutDown()
		}
	}

	// Если ждём — дождаться завершения воркеров
	if q.waiting {
		q.wg.Wait()

		// Пост-коррекция ёмкости: снимаем "хвост" (queued/delayed), который мог потеряться при остановке.
		// Оставляем резерв только под pending, т.к. они переживают рестарт.
		q.pendingMu.Lock()
		pend := uint64(len(q.pending))
		q.pendingMu.Unlock()

		cur := q.currentCapacity.Load()
		if cur > pend {
			q.unreserve(cur - pend)
		}
	} else {
		// waiting=false: не трогаем счётчик — in-flight сами вызовут dec() позже.
		// Для Stop хвост остаётся учтённым (задокументированная утечка до следующего корректного цикла).
	}

	// Финализируем состояние и публикуем отсутствие очереди.
	q.lifecycleMu.Lock()
	q.setState(stateStopped)
	q.lifecycleMu.Unlock()

	q.queueMu.Lock()
	q.queue = nil
	q.queueMu.Unlock()

	log.Printf(service + ": queue is drained/stopped")
}

func (q *RateEnvelopeQueue) tryReserve(n uint64) bool {
	for {
		cur := q.currentCapacity.Load()
		if q.allowedCapacity != 0 && cur+n > q.allowedCapacity {
			return false
		}
		if q.currentCapacity.CompareAndSwap(cur, cur+n) {
			return true
		}
	}
}

func (q *RateEnvelopeQueue) unreserve(n uint64) {
	q.currentCapacity.Add(^uint64(n - 1))
}

func (q *RateEnvelopeQueue) inc(n uint64) {
	q.currentCapacity.Add(n)
}

func (q *RateEnvelopeQueue) dec() {
	q.currentCapacity.Add(^uint64(0))
}

func WithLimitOption(limit int) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		q.limit = limit
	}
}

func WithWaitingOption(waiting bool) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		q.waiting = waiting
	}
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
			c := *conf
			q.workqueueConf = &c
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

func WithStamps(stamps ...Stamp) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		q.queueStamps = append(q.queueStamps, stamps...)
	}
}

func WithAllowedCapacityOption(capacity uint64) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		q.allowedCapacity = capacity
	}
}
