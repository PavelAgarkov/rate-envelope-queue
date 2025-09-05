# rate-envelope-queue

> A lightweight, goroutine-safe wrapper around `k8s.io/client-go/util/workqueue` for managing tasks (**envelopes**) with a fixed worker pool, periodic scheduling, deadlines, hooks, and **stamps (middleware)**. Adds a safe queue lifecycle (**Start/Stop/Start**), buffering before first start, and queue capacity limiting.

> Under the hood it uses `workqueue.TypedRateLimitingInterface`. Deduplication happens by **pointer** to the element: repeated `Add` of the *same pointer* while it is in-flight is ignored.

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Concepts & Contracts](#concepts--contracts)
    - [Envelope](#envelope)
    - [Queue](#queue)
    - [Stamps (middleware)](#stamps-middleware)
- [Worker Behavior](#worker-behavior)
- [Stop Modes](#stop-modes)
- [Capacity Limiting](#capacity-limiting)
- [Benchmarks](#benchmarks)
- [Metrics (Prometheus)](#metrics-prometheus)
- [Examples](#examples)
- [License](#license)

---

## Features

Goroutine-safe **in-memory** queue for your service:

- All public methods are safe to call from multiple goroutines. `Send` can be called concurrently. Multiple workers are supported via `limit`. `Start/Stop` are serialized internally.
- This is **not** a distributed queue: there are no guarantees across processes/hosts. Ensure your hooks/invokers are thread-safe around shared state.

What you get:

- A clear lifecycle FSM: `init → running → stopping → stopped`
- **Both one-off and periodic** tasks
- **Middleware chain** via `Stamp`
- **Hooks**: `before/after/failure/success`
- **Capacity accounting** (quota)
- **Graceful or fast stop** (`Drain` / `Stop`) and **restartable** queues (`Start()` after `Stop()`)

Error/Hook semantics:

- `ErrStopEnvelope` — intentional stop of a specific envelope:
    - the envelope is **forgotten**, **not** rescheduled;
    - if raised in `beforeHook`/`invoke`, the `afterHook` still runs (within a time-bounded context); `successHook` does **not** run.
- `context.Canceled` / `context.DeadlineExceeded` — not a success:
    - envelope is forgotten; periodic ones are rescheduled, one-off ones are not.
- Any other error:
    - **periodic** → rescheduled (if queue is alive);
    - **one-off** → defer to `failureHook` decision (`RetryNow` / `RetryAfter` / `Drop`).
- Each hook runs with its own timeout: a fraction `frac=0.5` of the envelope's `deadline`, but at least `hardHookLimit` (800ms). Hook timeouts are derived from the task context `tctx`, so hooks never outlive the envelope deadline.

Concurrency controls (brief):

- `stateMu` guards the FSM state (RLock read / Lock write)
- `lifecycleMu` serializes Start/Stop/queue swap
- `queueMu` guards the inner workqueue pointer
- `pendingMu` guards the pre-start buffer
- `run` is an atomic fast flag for “queue alive”
- Capacity accounting is atomic via `tryReserve/inc/dec/unreserve`

Other highlights:

- **Worker pool** via `WithLimitOption(n)`
- **Start/Stop/Start**: tasks sent before first start are buffered and flushed on `Start()`
- **Periodic vs one-off**: `interval > 0` means periodic; `interval == 0` means one-off
- **Deadlines**: `deadline > 0` bounds `invoke` time via `context.WithTimeout` in the worker
- **Stamps**: both global (queue-level) and per-envelope (task-level), with predictable execution order
- **Panic safety**: panics inside task are handled (`Forget+Done`) and logged with stack; worker keeps running
- **Prometheus metrics**: use `client-go` workqueue metrics

---

## Installation

```bash
go get github.com/PavelAgarkov/rate-envelope-queue
```

Recommended pins (compatible with this package):

```bash
go get k8s.io/client-go@v0.34.0
go get k8s.io/component-base@v0.34.0
```

**Requires:** Go **1.24+**.

---

## Quick Start

See full examples in [`examples/`](./examples):

- `queue_with_simple_start_stop_dynamic_execute.go`
- `simple_queue_with_simple_preset_envelopes.go`
- `simple_queue_with_simple_schedule_envelopes.go`
- `simple_queue_with_simple_dynamic_envelopes.go`
- `simple_queue_with_simple_combine_envelopes.go`

Capacity scenarios (accounting correctness):

**Drain + waiting=true** — wait for all workers; all `dec()` happen; no remainder.
```go
envelopeQueue := NewRateEnvelopeQueue(
    parent,
    "test_queue",
    WithLimitOption(5),
    WithWaitingOption(true),
    WithStopModeOption(Drain),
    WithAllowedCapacityOption(50),
)
```

**Stop + waiting=true** — after `wg.Wait()` we subtract the “tail” (`cur - pend`), the counter converges.
```go
envelopeQueue := NewRateEnvelopeQueue(
    parent,
    "test_queue",
    WithLimitOption(5),
    WithWaitingOption(true),
    WithStopModeOption(Stop),
    WithAllowedCapacityOption(50),
)
```

**Unlimited capacity** — `WithAllowedCapacityOption(0)` removes admission limits; the `currentCapacity` metric still reflects actual occupancy.
```go
envelopeQueue := NewRateEnvelopeQueue(
    parent,
    "test_queue",
    WithLimitOption(5),
    WithWaitingOption(true),
    WithStopModeOption(Drain),
    WithAllowedCapacityOption(0),
)
```

Minimal API sketch:
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

q := NewRateEnvelopeQueue(
    ctx,
    "emails",
    WithLimitOption(4),
    WithWaitingOption(true),
    WithStopModeOption(Drain),
    WithAllowedCapacityOption(1000),
    WithStamps(LoggingStamp()),
)

emailOnce, _ := NewEnvelope(
    WithId(1),
    WithType("email"),
    WithScheduleModeInterval(0),          // one-off
    WithDeadline(3*time.Second),
    WithInvoke(func(ctx context.Context, e *Envelope) error { return nil }),
)

ticker, _ := NewEnvelope(
    WithId(2),
    WithType("metrics"),
    WithScheduleModeInterval(5*time.Second), // periodic
    WithDeadline(2*time.Second),
    WithInvoke(func(ctx context.Context, e *Envelope) error { return nil }),
)

q.Start()
_ = q.Send(emailOnce, ticker)
// ...
q.Stop()
q.Start() // restart if needed
```

---

## Concepts & Contracts

### Envelope

```go
e, err := NewEnvelope(
    WithId(123), // optional, for logs
    WithType("my_task"), // optional, for logs
    WithScheduleModeInterval(time.Second), // 0 = one-off
    WithDeadline(500*time.Millisecond),    // 0 = no deadline
    WithBeforeHook(func(ctx context.Context, e *Envelope) error { return nil }),
    WithInvoke(func(ctx context.Context, e *Envelope) error { return nil }), // required
    WithAfterHook(func(ctx context.Context, e *Envelope) error { return nil }),
    WithFailureHook(func(ctx context.Context, e *Envelope, err error) Decision {
        return DefaultOnceDecision()               // Drop by default
        // return RetryOnceAfterDecision(5 * time.Second)
        // return RetryOnceNowDecision()
    }),
    WithSuccessHook(func(ctx context.Context, e *Envelope) {}),
    WithStampsPerEnvelope(LoggingStamp()),
    WithPayload(myPayload),
)
```

Validation:
- `invoke` is required; `interval >= 0`; `deadline >= 0`
- For periodic: `deadline` **must not exceed** `interval` → `ErrAdditionEnvelopeToQueueBadIntervals`

Special error:
- `ErrStopEnvelope` — gracefully stops **this envelope only** (no reschedule)

### Queue

```go
q := NewRateEnvelopeQueue(ctx, "queue-name",
    WithLimitOption(n),
    WithWaitingOption(true|false),
    WithStopModeOption(Drain|Stop),
    WithAllowedCapacityOption(cap),         // 0 = unlimited
    WithWorkqueueConfigOption(conf),
    WithLimiterOption(limiter),
    WithStamps(stamps...),
)
q.Start()
err := q.Send(e1, e2, e3)                   // ErrAllowedQueueCapacityExceeded on overflow
q.Stop()
```

**Pre-start buffer.** In `init`, `Send()` pushes envelopes into an internal buffer; on `Start()` they are flushed into the workqueue.

### Stamps (middleware)

```go
type (
    Invoker  func(ctx context.Context, envelope *Envelope) error
    Stamp    func(next Invoker) Invoker
)
```

Order: **global** stamps (outer) wrap **per-envelope** stamps (inner), then `Envelope.invoke`.

A sample `LoggingStamp()` is provided for demonstration.

---

## Worker Behavior

| Result / condition                      | Queue action                                                                         |
|-----------------------------------------|--------------------------------------------------------------------------------------|
| `invoke` returns `nil`                  | `Forget`; if `interval > 0` and alive → `AddAfter(interval)`                         |
| `context.Canceled` / `DeadlineExceeded` | `Forget`; if periodic and alive → `AddAfter(interval)`                               |
| `ErrStopEnvelope`                       | `Forget`; **no** reschedule                                                          |
| Error on **periodic**                   | `Forget`; if alive → `AddAfter(interval)`                                            |
| Error on **one-off** + `failureHook`    | Use decision: `RetryNow` / `RetryAfter(d)` / `Drop`                                  |
| Panic in task                           | `Forget + Done` + stack log; worker continues                                        |

> “Queue is alive” = `run == true`, state is `running`, base context not done, and `workqueue` not shutting down.

---

## Stop Modes

| Waiting \\ StopMode | `Drain` (graceful)                              | `Stop` (fast)                                   |
|---------------------|--------------------------------------------------|--------------------------------------------------|
| `true`              | Wait for workers; `ShutDownWithDrain()`         | Wait for workers; `ShutDown()`                   |
| `false`             | No wait; `ShutDownWithDrain()`                  | **Immediate** stop; `ShutDown()`                 |

After `Stop()` you can call `Start()` again: a fresh inner `workqueue` will be created.

---

## Capacity Limiting

`WithAllowedCapacityOption(cap)` limits the **total** number of in-flight/queued/delayed items (including reschedules).  
If the limit would be exceeded, `Send()` returns `ErrAllowedQueueCapacityExceeded`.  
`currentCapacity` is updated on add, reschedule, and completion.

- `cap == 0` → **unlimited** admission; the `currentCapacity` metric still tracks actual occupancy.
- `Stop + waiting=false + StopMode=Stop` — documented tail leakage in accounting. Use `Drain` or `waiting=true` for accurate capacity convergence.

---

## Benchmarks

Command examples:

```bash
go test -bench=BenchmarkQueueFull -benchmem
go test -bench=BenchmarkQueueInterval -benchmem
```

Numbers provided by the author (your CPU/env will vary):

```
BenchmarkQueueFull-8         3212882               348.7 ns/op            40 B/op          1 allocs/op
BenchmarkQueueInterval-8      110313             12903 ns/op            1809 B/op         24 allocs/op
```

---

## Metrics (Prometheus)

Workqueue metrics are enabled via blank import:

```go
import (
    _ "k8s.io/component-base/metrics/prometheus/workqueue"
    "k8s.io/component-base/metrics/legacyregistry"
    "net/http"
)

func serveMetrics() {
    mux := http.NewServeMux()
    mux.Handle("/metrics", legacyregistry.Handler())
    go http.ListenAndServe(":8080", mux)
}
```

Your queue name (`QueueConfig.Name`) is included in workqueue metric labels (`workqueue_*`: adds, depth, work_duration, retries, etc.).

---

## Examples

See the [`examples/`](./examples) folder for runnable snippets covering one-off jobs, periodic schedules, combined modes, and dynamic dispatch.

---

## License

MIT — see [`LICENSE`](./LICENSE).

---

---

# rate-envelope-queue (Русская версия)

> Лёгкая, потокобезопасная обёртка над `k8s.io/client-go/util/workqueue` для управления задачами (**envelopes**) с фиксированным пулом воркеров, периодическим планированием, дедлайнами, хуками и **stamps (middleware)**. Добавляет безопасный жизненный цикл очереди (**Start/Stop/Start**), буферизацию задач до старта и ограничение ёмкости очереди.

> В основе — `workqueue.TypedRateLimitingInterface`. Дедупликация происходит по **указателю** на элемент: повторный `Add` того же *указателя* пока он в обработке — игнорируется.

---

## Содержание

- [Ключевые возможности](#ключевые-возможности)
- [Установка](#установка)
- [Быстрый старт](#быстрый-старт)
- [Концепции и контракты](#концепции-и-контракты)
    - [Envelope](#envelope-1)
    - [Очередь](#очередь)
    - [Stamps (middleware)](#stamps-middleware-1)
- [Поведение воркера](#поведение-воркера)
- [Режимы остановки](#режимы-остановки)
- [Ограничение ёмкости](#ограничение-ёмкости)
- [Бенчмарки](#бенчмарки)
- [Метрики (Prometheus)](#метрики-prometheus)
- [Примеры](#примеры)
- [Лицензия](#лицензия-1)

---

## Ключевые возможности

Потокобезопасная локальная очередь в памяти приложения:

- Все публичные методы безопасны при вызовах из нескольких горутин. `Send` можно вызывать параллельно. Воркеров может быть несколько (`limit`). Вызовы `Start/Stop` сериализуются внутри.
- Это **не** распределённая очередь: гарантий между разными процессами/узлами нет. Код хуков/инвокеров должен сам обеспечивать потокобезопасность при доступе к общим ресурсам.

Что внутри:

- Прозрачный автомат состояний: `init → running → stopping → stopped`
- **Одноразовые и периодические** задачи
- **Цепочка middleware** через `Stamp`
- **Хуки**: `before/after/failure/success`
- **Учёт ёмкости** (quota)
- **Мягкий или быстрый останов** (`Drain` / `Stop`) и **повторный старт** (`Start()` после `Stop()`)

Семантика ошибок и хуков:

- `ErrStopEnvelope` — намеренная остановка конкретного конверта:
    - конверт **забывается**, **не** перепланируется;
    - если ошибка возникла в `beforeHook`/`invoke`, `afterHook` всё равно вызовется (с ограниченным временем); `successHook` **не** вызывается.
- `context.Canceled` / `context.DeadlineExceeded` — это **не** успех:
    - конверт забывается; периодический — перепланируется, одноразовый — нет.
- Любая другая ошибка:
    - **периодическая** → перепланируется (если очередь «жива»);
    - **одноразовая** → решение через `failureHook` (`RetryNow` / `RetryAfter` / `Drop`).
- Каждый хук выполняется с собственным таймаутом: доля `frac=0.5` от `deadline` конверта, но не меньше `hardHookLimit` (800мс). Таймауты «висят» на `tctx`, т.е. хуки никогда не переживут дедлайн конверта.

Потокобезопасность (коротко):

- `stateMu` — чтение/запись состояния
- `lifecycleMu` — сериализация Start/Stop/смены очереди
- `queueMu` — доступ к внутренней очереди
- `pendingMu` — буфер задач до старта
- `run` — атомарный флаг «жива ли очередь»
- Учёт ёмкости — атомарные операции `tryReserve/inc/dec/unreserve`

Прочее:

- **Пул воркеров**: `WithLimitOption(n)`
- **Start/Stop/Start**: задачи, добавленные до первого `Start()`, буферизуются и переливаются в очередь при старте
- **Периодические / одноразовые**: `interval > 0` — периодическая; `interval == 0` — одноразовая
- **Дедлайны**: `deadline > 0` ограничивает время `invoke` через `context.WithTimeout`
- **Stamps**: глобальные и на уровне конверта, порядок выполнения предсказуем
- **Защита от паник**: паника в задаче → `Forget+Done` и лог стека; воркер продолжает работу
- **Метрики Prometheus**: из `client-go` workqueue

---

## Установка

```bash
go get github.com/PavelAgarkov/rate-envelope-queue
```

Рекомендуемые версии:

```bash
go get k8s.io/client-go@v0.34.0
go get k8s.io/component-base@v0.34.0
```

**Требования:** Go **1.24+**.

---

## Быстрый старт

Смотрите каталог [`examples/`](./examples):

- `queue_with_simple_start_stop_dynamic_execute.go`
- `simple_queue_with_simple_preset_envelopes.go`
- `simple_queue_with_simple_schedule_envelopes.go`
- `simple_queue_with_simple_dynamic_envelopes.go`
- `simple_queue_with_simple_combine_envelopes.go`

Сценарии ёмкости (корректность учёта):

**Drain + waiting=true** — дожидаемся всех воркеров; все `dec()` прошли; остатка нет.
```go
envelopeQueue := NewRateEnvelopeQueue(
    parent,
    "test_queue",
    WithLimitOption(5),
    WithWaitingOption(true),
    WithStopModeOption(Drain),
    WithAllowedCapacityOption(50),
)
```

**Stop + waiting=true** — после `wg.Wait()` снимается «хвост» (`cur - pend`), счётчик сходится.
```go
envelopeQueue := NewRateEnvelopeQueue(
    parent,
    "test_queue",
    WithLimitOption(5),
    WithWaitingOption(true),
    WithStopModeOption(Stop),
    WithAllowedCapacityOption(50),
)
```

**Безлимитная ёмкость** — `WithAllowedCapacityOption(0)` убирает ограничение приёма; метрика `currentCapacity` отражает фактическую занятость.
```go
envelopeQueue := NewRateEnvelopeQueue(
    parent,
    "test_queue",
    WithLimitOption(5),
    WithWaitingOption(true),
    WithStopModeOption(Drain),
    WithAllowedCapacityOption(0),
)
```

Мини‑пример:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// тут будут другие примеры
q := NewRateEnvelopeQueue(
    ctx,
    "emails",
    WithLimitOption(4),
    WithWaitingOption(true),
    WithStopModeOption(Drain),
    WithAllowedCapacityOption(1000),
    WithStamps(LoggingStamp()),
)

emailOnce, _ := NewEnvelope(
    WithId(1),
    WithType("email"),
    WithScheduleModeInterval(0),          // одноразовая
    WithDeadline(3*time.Second),
    WithInvoke(func(ctx context.Context, e *Envelope) error { return nil }),
)

ticker, _ := NewEnvelope(
    WithId(2),
    WithType("metrics"),
    WithScheduleModeInterval(5*time.Second), // периодическая
    WithDeadline(2*time.Second),
    WithInvoke(func(ctx context.Context, e *Envelope) error { return nil }),
)

q.Start()
_ = q.Send(emailOnce, ticker)
// ...
q.Stop()
q.Start() // при необходимости можно снова стартовать
```

---

## Концепции и контракты

### Envelope

```go
e, err := NewEnvelope(
    WithId(123),                   // опционально, для логов
    WithType("my_task"),           // опционально, для логов
    WithScheduleModeInterval(time.Second), // 0 = одноразовая
    WithDeadline(500*time.Millisecond),    // 0 = без дедлайна
    WithBeforeHook(func(ctx context.Context, e *Envelope) error { return nil }),
    WithInvoke(func(ctx context.Context, e *Envelope) error { return nil }), // обязательно
    WithAfterHook(func(ctx context.Context, e *Envelope) error { return nil }),
    WithFailureHook(func(ctx context.Context, e *Envelope, err error) Decision {
        return DefaultOnceDecision()               // по умолчанию Drop
        // return RetryOnceAfterDecision(5 * time.Second)
        // return RetryOnceNowDecision()
    }),
    WithSuccessHook(func(ctx context.Context, e *Envelope) {}),
    WithStampsPerEnvelope(LoggingStamp()),
    WithPayload(myPayload),
)
```

Валидация:
- `invoke` обязателен; `interval >= 0`; `deadline >= 0`
- Для периодических: `deadline` **не должен превышать** `interval` → `ErrAdditionEnvelopeToQueueBadIntervals`

Спец‑ошибка:
- `ErrStopEnvelope` — корректно прекращает **только этот конверт** (без перепланирования)

### Очередь

```go
q := NewRateEnvelopeQueue(ctx, "queue-name",
    WithLimitOption(n),
    WithWaitingOption(true|false),
    WithStopModeOption(Drain|Stop),
    WithAllowedCapacityOption(cap),         // 0 = без лимита
    WithWorkqueueConfigOption(conf),
    WithLimiterOption(limiter),
    WithStamps(stamps...),
)
q.Start()
err := q.Send(e1, e2, e3)                   // ErrAllowedQueueCapacityExceeded при переполнении
q.Stop()
```

**Буфер до старта.** В состоянии `init` `Send()` складывает задачи во внутренний буфер; при `Start()` — они переливаются в `workqueue`.

### Stamps (middleware)

```go
type (
    Invoker  func(ctx context.Context, envelope *Envelope) error
    Stamp    func(next Invoker) Invoker
)
```

Порядок: **сначала глобальные** stamps (внешние), затем **per‑envelope** (внутренние), после — `Envelope.invoke`.

`LoggingStamp()` — пример для иллюстрации (не «серебряная пуля» для продакшена).

---

## Поведение воркера

| Событие / результат                     | Действие очереди                                                                     |
|-----------------------------------------|--------------------------------------------------------------------------------------|
| `invoke` вернул `nil`                   | `Forget`; если `interval > 0` и очередь «жива» → `AddAfter(interval)`                |
| `context.Canceled` / `DeadlineExceeded` | `Forget`; если периодическая и очередь «жива» → `AddAfter(interval)`                 |
| `ErrStopEnvelope`                       | `Forget`; **не** перепланируем                                                       |
| Ошибка у **периодической**              | `Forget`; если очередь «жива» → `AddAfter(interval)`                                 |
| Ошибка у **одноразовой** + `failureHook`| Решение пользователя: `RetryNow` / `RetryAfter(d)` / `Drop`                          |
| Паника в задаче                         | `Forget + Done` + лог стека; воркер продолжает работу                                |

> «Очередь жива» = `run == true`, `state == running`, базовый контекст не завершён и `workqueue` не в shutdown.

---

## Режимы остановки

| Waiting \\ StopMode | `Drain` (мягкая)                                   | `Stop` (жёсткая)                           |
|---------------------|-----------------------------------------------------|--------------------------------------------|
| `true`              | Ждать воркеров; `ShutDownWithDrain()`               | Ждать воркеров; `ShutDown()`               |
| `false`             | Без ожидания воркеров; `ShutDownWithDrain()`        | **Мгновенный** останов; `ShutDown()`       |

После `Stop()` можно вызывать `Start()` повторно: создаётся новый внутренний `workqueue`.

---

## Ограничение ёмкости

`WithAllowedCapacityOption(cap)` ограничивает суммарное число элементов в системе (включая перепланированные).  
При попытке превышения лимита `Send()` возвращает `ErrAllowedQueueCapacityExceeded`.  
`currentCapacity` обновляется при добавлении, перепланировании и завершении обработки.

- `cap == 0` → **безлимит** по приёму; метрика `currentCapacity` отражает фактическую занятость.
- `Stop + waiting=false + StopMode=Stop` — документированная утечка «хвоста» в учёте. Для точной сходимости используйте `Drain` или `waiting=true`.

---

## Бенчмарки

Как запускать:

```bash
go test -bench=BenchmarkQueueFull -benchmem
go test -bench=BenchmarkQueueInterval -benchmem
```

Цифры автора (зависят от CPU/окружения):

```
BenchmarkQueueFull-8         3212882               348.7 ns/op            40 B/op          1 allocs/op
BenchmarkQueueInterval-8      110313             12903 ns/op            1809 B/op         24 allocs/op
```

---

## Метрики (Prometheus)

Метрики `workqueue` активируются бланк‑импортом:

```go
import (
    _ "k8s.io/component-base/metrics/prometheus/workqueue"
    "k8s.io/component-base/metrics/legacyregistry"
    "net/http"
)

func serveMetrics() {
    mux := http.NewServeMux()
    mux.Handle("/metrics", legacyregistry.Handler())
    go http.ListenAndServe(":8080", mux)
}
```

Имя очереди (`QueueConfig.Name`) попадает в лейблы метрик (`workqueue_*`: adds, depth, work_duration, retries и т.д.).

---

## Примеры

Смотрите каталог [`examples/`](./examples) — там есть готовые варианты для одноразовых задач, периодических расписаний, комбинированных сценариев и динамического диспатча.

---

## Лицензия

MIT — см. [`LICENSE`](./LICENSE).