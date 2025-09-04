# rate-envelope-queue

Лёгкая обёртка над `k8s.io/client-go/util/workqueue` для управления задачами (**envelopes**) с фиксированным пулом воркеров, периодическим планированием, дедлайнами, хуками и **stamps (middleware)**. Добавляет безопасный жизненный цикл очереди (**Start/Stop/Start**), буферизацию задач до старта и ограничение ёмкости очереди.

> В основе — `workqueue.TypedRateLimitingInterface`. Дедупликация происходит по **указателю** на элемент: повторный `Add` того же *указателя* пока он в обработке — игнорируется.

---

## Содержание

- [Ключевые возможности](#ключевые-возможности)
- [Установка](#установка)
- [Быстрый старт](#быстрый-старт)
- [Концепции и контракты](#концепции-и-контракты)
    - [Envelope](#envelope)
    - [Очередь](#очередь)
    - [Stamps (middleware)](#stamps-middleware)
- [Поведение воркера](#поведение-воркера)
- [Режимы остановки](#режимы-остановки)
- [Ограничение ёмкости](#ограничение-ёмкости)
- [Бенчмарки](#бенчмарки)
- [Метрики (Prometheus)](#метрики-prometheus)
- [Примеры](#примеры)
- [Лицензия](#лицензия)

---

## Ключевые возможности

- **Пул воркеров**: настраивается через `WithLimitOption(n)`.
- **Start/Stop/Start**: очередь можно **перезапускать**; задачи, добавленные до первого `Start()`, буферизуются и заливаются в очередь при старте.
- **Периодические / одноразовые задачи**: `interval > 0` → периодическая; `interval == 0` → одноразовая.
- **Дедлайны**: `deadline > 0` ограничивает время исполнения `invoke` (через `context.WithTimeout` в воркере).
- **Хуки**: `beforeHook`, `afterHook`, `failureHook`, `successHook`.
- **Stamps (middleware)**: глобальные (для очереди) и per-envelope (для задачи) с предсказуемым порядком выполнения.
- **Ограничение ёмкости**: `WithAllowedCapacityOption(cap)` — защита от переполнения очереди; `Send()` вернёт `ErrAllowedQueueCapacityExceeded`, если лимит превышен.
- **Безопасность при паниках**: паника внутри задачи → `Forget+Done` и лог со стеком; воркер не падает.
- **Метрики workqueue**: экспорт Prometheus метрик из `client-go`.

---

## Установка

```bash
go get github.com/PavelAgarkov/rate-envelope-queue
```

Рекомендуемые пин-версии (совместимы с кодом):

```bash
go get k8s.io/client-go@v0.34.0
go get k8s.io/component-base@v0.34.0
```

**Требования:** Go **1.24+**.

---

## Быстрый старт

Полные варианты, в том числе с облегченным интерфейсом, смотрите в каталоге [`examples/`](./examples):

- `queue_with_simple_start_stop_dynamic_execute.go`
- `simple_queue_with_simple_preset_envelopes.go`
- `simple_queue_with_simple_schedule_envelopes.go`
- `simple_queue_with_simple_dynamic_envelopes.go`
- `simple_queue_with_simple_combine_envelopes.go`

Мини‑пример (схема API):

Коротко: текущая схема учёта емкости корректна для:
Drain + waiting=true — корректно: дожидаемся всех воркеров, все dec() проходят, остатка нет.
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

Stop + waiting=true — корректно: после wg.Wait() снимается резерв «хвоста» (cur - pend), счётчик сходится.
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

Безлимитная ёмкость — при WithAllowedCapacityOption(0) ограничений на приём нет, но метрика currentCapacity продолжает отражать фактическую занятость.
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
    WithId(123), // опционально, для логов
    WithType("my_task"), // опционально, для логов
    WithScheduleModeInterval(time.Second), // 0 = одноразовая
    WithDeadline(500*time.Millisecond), // 0 = без дедлайна
    WithBeforeHook(func(ctx context.Context, e *Envelope) error { return nil }), // до invoke
    WithInvoke(func(ctx context.Context, e *Envelope) error { return nil }), // обязательно
    WithAfterHook(func(ctx context.Context, e *Envelope) error { return nil }), // после invoke
    WithFailureHook(func(ctx context.Context, e *Envelope, err error) Decision { // при ошибке в invoke сюда падают ошибки и envelope
        return DefaultOnceDecision() // drop
		//return RetryOnceAfterDecision(5 * time.Second) // перепланировать через 5 секунд
        //return RetryOnceNowDecision() // перепланировать сразу
    }),
    WithSuccessHook(func(ctx context.Context, e *Envelope) {}), // при успехе
    WithStampsPerEnvelope(LoggingStamp()), // штампы только для этой задачи
    WithPayload(myPayload), // опционально, для пользовательских данных
)
```

Валидация:
- `invoke` обязателен; `interval >= 0`; `deadline >= 0`.
- Для периодических задач: `deadline` **не должен превышать** `interval` → `ErrAdditionEnvelopeToQueueBadIntervals`.

Спец‑ошибка:
- `ErrStopEnvelope` — корректно прекращает **только этот конверт** (без перепланирования).

### Очередь

```go
q := NewRateEnvelopeQueue(ctx, "queue-name",
    WithLimitOption(n), // число воркеров > 0
    WithWaitingOption(true|false), // ждать воркеров при остановке или нет
    WithStopModeOption(Drain|Stop), // мягкая или жёсткая остановка
    WithAllowedCapacityOption(cap),         // 0 = без лимита
    WithWorkqueueConfigOption(conf),        // workqueue.TypedRateLimitingQueueConfig
    WithLimiterOption(limiter),             // свой rate limiter при необходимости
    WithStamps(stamps...),                  // глобальные stamps (наружные)
)
q.Start()
err := q.Send(e1, e2, e3)                   // ErrAllowedQueueCapacityExceeded при переполнении
q.Stop()
```

**Буфер до старта.** В состоянии `init` вызовы `Send()` складывают задачи во внутренний буфер; при `Start()` — они переливаются в `workqueue`.

### Stamps (middleware)

```go
type (
    Invoker  func(ctx context.Context, envelope *Envelope) error
    Stamp    func(next Invoker) Invoker
)
```

Порядок: **сначала глобальные** stamps (внешние), затем **per‑envelope** (внутренние), после чего — `Envelope.invoke`.

В комплекте есть `LoggingStamp()`. - Скорее для примера, чем для продакшена.

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

> «Очередь жива» = флаг `run == true`, `state == running`, базовый контекст не завершён и `workqueue` не в shutdown.

---

## Режимы остановки

| Waiting \ StopMode | `Drain` (мягкая)                                   | `Stop` (жёсткая)                           |
|---------------------|-----------------------------------------------------|--------------------------------------------|
| `true`              | Ждать воркеров; `ShutDownWithDrain()`               | Ждать воркеров; `ShutDown()`               |
| `false`             | Без ожидания воркеров; `ShutDownWithDrain()`        | **Мгновенная** остановка; `ShutDown()`     |

После `Stop()` можно вызывать `Start()` повторно: создаётся новый внутренний `workqueue`.

---

## Ограничение ёмкости

`WithAllowedCapacityOption(cap)` — ограничивает суммарное количество элементов в очереди (включая перепланированные).  
При попытке превышения лимита `Send()` вернёт `ErrAllowedQueueCapacityExceeded`.  
Счётчик текущей ёмкости обновляется при добавлении, перепланировании и завершении обработки.

---

## Бенчмарки

Повторяемые команды:

```bash
go test -bench=BenchmarkQueueFull -benchmem
go test -bench=BenchmarkQueueInterval -benchmem
```

Результаты, предоставленные пользователем (окружение/CPU могут влиять на цифры):

```
$ go test -bench=BenchmarkQueueFull -benchmem
  4874815               315.0 ns/op            18 B/op          1 allocs/op
PASS
ok      github.com/PavelAgarkov/rate-envelope-queue     1.796s
```

```
$ go test -bench=BenchmarkQueueInterval -benchmem
    97928             13335 ns/op            1715 B/op         22 allocs/op
PASS
ok      github.com/PavelAgarkov/rate-envelope-queue     2.297s
```

Краткая интерпретация:
- **`BenchmarkQueueFull`** — добавление одноразового конверта с пустыми хуками: ~315 нс/операцию, 1 аллокация.
- **`BenchmarkQueueInterval`** — активная перепланировка (`AddAfter`) множества периодических задач: ~13.3 мкс/операцию, ожидаемо больше аллокаций за счёт таймеров и внутренних структур.

---

## Метрики (Prometheus)

Метрики `workqueue` регистрируются бланк‑импортом:

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

## Лицензия

MIT — см. [`LICENSE`](./LICENSE).