# rate-envelope-queue

Лёгкий пакет для управления пулом задач (**envelopes**) поверх `k8s.io/client-go/util/workqueue` с ограничением параллелизма, ретраями, периодическим планированием, **stamps (middleware)** и хуками до/после выполнения.

> Основано на `workqueue` из client-go: очередь дедуплицирует одинаковые элементы (один и тот же **указатель**) и поддерживает rate-limiting / отложенное перепланирование.

---

## Возможности

- **Фиксированный пул воркеров**: настраиваемый параллелизм через `WithLimitOption`.
- **Периодические и одноразовые задачи**: `interval > 0` → периодические; `interval == 0` → одноразовые.
- **Дедлайны**: `deadline > 0` ограничивает время выполнения `invoke` (в воркере оборачивается таймаутом).
- **Хуки**: `beforeHook` / `afterHook` с отдельным тайм‑бюджетом (по умолчанию `max(20% от deadline, 800ms)`), задаётся через `WithHookTimeout`.
- **stamps (middleware)**:
    - **Глобальные** (для всей очереди) — задаются через `WithStamps(...)`.
    - **Per‑envelope** — в `Envelope.stamps`.
    - Порядок компоновки: **сначала глобальные, затем per‑envelope** — глобальные оказываются **внешними**.
- **Семантика остановки типа**: верните `ErrStopEnvelope` из `beforeHook`/`invoke`/`afterHook`, чтобы поместить `Envelope._type` в blacklist — все будущие задачи этого типа игнорируются.
- **Ретраи и backoff**: дефолтный лимитер = `MaxOf(Exponential(1s..30s), TokenBucket(5 rps, burst=10))`.
- **Грациозная остановка**: режимы `Drain` (дождаться завершения) и `Stop` (остановить сразу).
- **Безопасность при паниках**: паника в обработке **конверта** перехватывается, элемент `Forget+Done`, стек логируется; паника уровня воркера тоже перехватывается.

---

## Требования

- Go рекомендуется **1.22+** (`atomic.Bool`).
- Модули:
    - `k8s.io/client-go/util/workqueue`
    - `golang.org/x/time/rate`
---

## Установка

```bash
go get github.com/PavelAgarkov/rate-pool/pkg
```

```go
import "github.com/PavelAgarkov/rate-pool/pkg"
```

---

## Быстрый старт

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

q := pkg.NewRateEnvelopeQueue(
    pkg.WithLimitOption(3),           // 3 воркера
    pkg.WithWaitingOption(true),      // ждать завершения горутин при Stop()
    pkg.WithStopModeOption(pkg.Drain),
    pkg.WithStamps(                   // глобальные stamps (внешние)
        pkg.LoggingStamp(log.Default()),
        pkg.BeforeAfterStamp(pkg.WithHookTimeout),
    ),
)

// периодическая задача
metrics := &pkg.Envelope{
    id:       2,
    _type:     "metrics",
    interval: 3 * time.Second,
    deadline: 1 * time.Second,
    invoke: func(ctx context.Context) error {
        fmt.Println("📊 Metrics", time.Now())
        return nil
    },
}

// одноразовая задача + per-envelope stamps (внутренние)
email := &pkg.Envelope{
    id:       1,
    _type:     "email",
    interval: 0,
    deadline: 2 * time.Second,
    invoke: func(ctx context.Context) error {
        // уважайте ctx.Done()
        return nil
    },
    beforeHook: func(ctx context.Context, e *pkg.Envelope) error {
        return nil
    },
    afterHook: func(ctx context.Context, e *pkg.Envelope) error {
        return nil
    },
    stamps: []pkg.Stamp{
        // свои пер-задачные обёртки (внутренние относительно глобальных)
    },
}

q.Start(ctx)
_ = q.Add(metrics, email)

time.AfterFunc(10*time.Second, cancel)
<-ctx.Done()

q.Stop()
```

---

## Поведение очереди

| Сценарий                                                         | Действие очереди                                                                 |
|------------------------------------------------------------------|----------------------------------------------------------------------------------|
| `invoke` вернул `nil`                                            | `Forget`; если `interval > 0` → `AddAfter(interval)`                             |
| Контекст задачи истёк/отменён (`DeadlineExceeded`/`Canceled`)    | `Forget`; если периодическая → `AddAfter(interval)`                              |
| `ErrStopEnvelope` (из `beforeHook`/`invoke`/`afterHook`)         | `Forget` + поместить `_type` в **blacklist**                                      |
| Ошибка в `beforeHook` (не `ErrStopEnvelope`)                     | Периодические: `AddRateLimited`; одноразовые: `Forget`                           |
| Ошибка в `invoke` (не `ErrStopEnvelope`)                         | Периодические: `AddRateLimited`; одноразовые: `Forget`                           |
| Ошибка в `afterHook` (не `ErrStopEnvelope`)                      | Возвращается наверх → те же правила, что и для обычной ошибки                   |
| Паника внутри обработки элемента                                 | Элемент `Forget+Done`, стек логируется; воркер продолжает работу                 |

> Валидация: для периодических задач `deadline` **не должен превышать** `interval` — иначе `ErrAdditionEnvelopeToQueueBadIntervals`.

---

## stamps (middleware)

stamps — это лёгкие обёртки вокруг `Invoker` (обработчика конверта). Их две группы:

- **Глобальные stamps** — задаются на очередь через `WithStamps(...)`.
- **Per‑envelope stamps** — задаются конкретной задачей в `Envelope.stamps`.

Порядок: глобальные идут **первее** и становятся **внешними** (самыми «оборачивающими»), затем per‑envelope — **внутренние**.

### Встроенные stamps

- `LoggingStamp(l *log.Logger)` — логирует длительность и ошибку обработки конверта.
- `BeforeAfterStamp(withTimeout)` — исполняет `beforeHook` и `afterHook` с отдельными тайм-бюджетами; любые ошибки, кроме `ErrStopEnvelope`, **возвращаются** наверх. Рекомендуемая функция тайм-бюджета:  
  `WithHookTimeout(ctx, base=deadline, frac=0.2, min=800ms)` → `max(20% от deadline, 800ms)`.

> Можно добавлять свои stamps для метрик/трейсинга/ограничения ресурсов.

---

## API (сокращённо)

```go
type StopMode string

const (
    Drain StopMode = "drain" // грациозная остановка (drain)
    Stop  StopMode = "stop"  // немедленная остановка
)

type Envelope struct {
    id       uint64
    _type     string
    interval time.Duration // 0 = одноразовая задача
    deadline time.Duration // 0 = без таймаута

    beforeHook func(ctx context.Context, envelope *Envelope) error
    invoke     func(ctx context.Context) error
    afterHook  func(ctx context.Context, envelope *Envelope) error

    stamps []Stamp // per-envelope stamps (внутренние)
}

type QueuePool interface {
    Start(ctx context.Context)
    Add(envelopes ...*Envelope) error
    Stop()
}

// stamps
type Invoker func(ctx context.Context, envelope *Envelope) error
type Stamp   func(next Invoker) Invoker

// конструктор
func NewRateEnvelopeQueue(options ...func(*RateEnvelopeQueue)) QueuePool
```

### Опции конструктора

```go
pkg.WithLimitOption(n)                 // число воркеров (>0)
pkg.WithWaitingOption(true|false)      // ждать ли завершения воркеров в Stop()
pkg.WithStopModeOption(pkg.Drain|pkg.Stop)
pkg.WithLimiterOption(customLimiter)   // если не задан — дефолтный
pkg.WithWorkqueueConfigOption(conf)    // конфиг workqueue (например, имя для метрик)
pkg.WithStamps(stamps...)              // глобальные stamps
```

**Дефолтный rate-limiter**:  
`MaxOf(ItemExponentialFailureRateLimiter(1s..30s), BucketRateLimiter(5 rps, burst=10))`.

### Ошибки

- `ErrStopEnvelope` — поместить `_type` в blacklist.
- `ErrEnvelopeInBlacklist` — попытка добавить envelope с типом из blacklist.
- `ErrEnvelopeQueueIsNotRunning` — `Add` до `Start`/после `Stop`.
- `ErrAdditionEnvelopeToQueueBadFields` — неверные поля (`_type`, `invoke`, `interval`, `deadline`).
- `ErrAdditionEnvelopeToQueueBadIntervals` — `deadline > interval` для периодических.

---

## Пример из теста (адаптирован)

```go
func Test_Acceptance(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    logger := log.New(os.Stdout, "", log.LstdFlags)

    email := &pkg.Envelope{
        id:       1,
        _type:     "email",
        interval: 5 * time.Second,
        deadline: 3 * time.Second,
        invoke: func(ctx context.Context) error {
            time.Sleep(5 * time.Second) // превысит дедлайн
            fmt.Println("📧 Email v1", time.Now())
            return nil
        },
        beforeHook: func(ctx context.Context, e *pkg.Envelope) error {
            fmt.Println("hook before email", e.id, time.Now())
            return nil
        },
        afterHook: func(ctx context.Context, e *pkg.Envelope) error {
            fmt.Println("hook after email", e.id, time.Now())
            // остановим дальнейшие email
            return pkg.ErrStopEnvelope
        },
        stamps: []pkg.Stamp{
            pkg.LoggingStamp(logger),
            pkg.BeforeAfterStamp(pkg.WithHookTimeout),
        },
    }

    metrics := &pkg.Envelope{
        id:       2,
        _type:     "metrics",
        interval: 3 * time.Second,
        deadline: 1 * time.Second,
        invoke: func(ctx context.Context) error {
            fmt.Println("📊 Metrics", time.Now())
            return nil
        },
    }

    food := &pkg.Envelope{
        id:       3,
        _type:     "food",
        interval: 2 * time.Second,
        deadline: 1 * time.Second,
        invoke: func(ctx context.Context) error {
            fmt.Println("🍔 Fooding", time.Now())
            return nil
        },
    }

    q := pkg.NewRateEnvelopeQueue(
        pkg.WithLimitOption(3),
        pkg.WithWaitingOption(true),
        pkg.WithStopModeOption(pkg.Drain),
    )

    q.Start(ctx)
    _ = q.Add(email, metrics, food, email) // повтор email будет дедуплицирован
    time.AfterFunc(25*time.Second, cancel)
    <-ctx.Done()
    q.Stop()
}
```

---

## Эксплуатационные заметки

- **Один объект — один запуск**: текущая реализация рассчитана на одноразовый жизненный цикл `Start/Stop`. Для повторного использования создайте **новый объект** очереди.
- **Дедупликация**: для указателей — по адресу. Не «переиспользуйте» один и тот же указатель для разных логических задач.
- **Jitter**: чтобы периодические задачи не «стреляли строем», можно добавить случайный сдвиг к `AddAfter`.
- **Соблюдайте контекст** в `invoke`/хуках: долгие операции должны уважать `ctx.Done()`; иначе получится «карусель» таймаутов с перепланированием.

---

## Лицензия

MIT

---

**Префикс логов:** `"[rate-envelope-queue]"`.