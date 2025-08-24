# rate-envelope-queue

Лёгкий пакет для управления пулом задач (**envelopes**) поверх `k8s.io/client-go/util/workqueue` с ограничением параллелизма, ретраями, периодическим планированием, **stamps (middleware)** и хуками до/после выполнения.

> Основано на `workqueue` из client-go: очередь дедуплицирует одинаковые элементы (один и тот же **указатель**) и поддерживает rate-limiting / отложенное перепланирование.

---

## Что нового в API

- **Builder-подход для Envelope** — поля неэкспортируемые, настройка через `NewEnvelope(opts...)` и `With*`-опции:
  ```go
  e := pkg.NewEnvelope(
      pkg.WithId(1),
      pkg.WithType("email"),
      pkg.WithInterval(5*time.Second),
      pkg.WithDeadline(3*time.Second),
      pkg.WithBeforeHook(func(ctx context.Context, e *pkg.Envelope) error { return nil }),
      pkg.WithInvoke(func(ctx context.Context) error { return nil }),
      pkg.WithAfterHook(func(ctx context.Context, e *pkg.Envelope) error { return nil }),
      pkg.WithStampsPerEnvelope(/* per-envelope stamps */),
  )
  ```
  Для чтения используйте геттеры: `GetId()`, `GetType()`, `GetStamps()`.

- **Stamps разделены на глобальные и per-envelope**:
    - Глобальные — через `WithStamps(...)` в конструкторе очереди.
    - Per-envelope — через `WithStampsPerEnvelope(...)` в `NewEnvelope(...)`.
    - Порядок исполнения: **сначала глобальные, затем per-envelope** (глобальные — внешние).

- **Тайм‑бюджеты для хуков** в `BeforeAfterStamp`: по умолчанию рекомендуем `frac=0.5` и `min=800ms` → `max(50% от deadline, 800ms)`.

---

## Возможности

- **Фиксированный пул воркеров**: параллелизм через `WithLimitOption`.
- **Периодические и одноразовые задачи**: `interval > 0` → периодические; `interval == 0` → одноразовые.
- **Дедлайны**: `deadline > 0` ограничивает время выполнения `invoke` (оборачивается таймаутом в воркере).
- **Хуки**: `beforeHook` / `afterHook` с отдельным тайм-бюджетом (через `BeforeAfterStamp(WithHookTimeout)`).
- **Stamps (middleware)**: глобальные и per-envelope; компонуются в цепочку (**chain**).
- **Остановка типа**: `ErrStopEnvelope` из любого места (`beforeHook`/`invoke`/`afterHook`) кладёт `_type` в **blacklist**.
- **Backoff/ретраи**: дефолтный лимитер = `MaxOf(Exponential(1s..30s), TokenBucket(5 rps, burst=10))`.
- **Грациозная остановка**: режимы `Drain`/`Stop`.
- **Безопасность при паниках**: паника внутри обработки **конверта** → `Forget+Done` и лог стека; паника воркера также перехватывается.

---

## Требования

- Go Рекомендовано **1.22+** (`atomic.Bool`).
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
        pkg.BeforeAfterStamp(pkg.WithHookTimeout), // 50% от deadline, минимум 800ms
        pkg.LoggingStamp(log.Default()),
    ),
)

email := pkg.NewEnvelope(
    pkg.WithId(1),
    pkg.WithType("email"),
    pkg.WithInterval(5*time.Second),
    pkg.WithDeadline(3*time.Second),
    pkg.WithBeforeHook(func(ctx context.Context, e *pkg.Envelope) error {
        fmt.Println("before:", e.GetId(), time.Now())
        return nil
    }),
    pkg.WithInvoke(func(ctx context.Context) error {
        // имитируем работу; уважайте ctx.Done()
        time.Sleep(5 * time.Second)
        fmt.Println("invoke email", time.Now())
        return nil
    }),
    pkg.WithAfterHook(func(ctx context.Context, e *pkg.Envelope) error {
        fmt.Println("after:", e.GetId(), time.Now())
        // Остановим дальнейшие email
        return pkg.ErrStopEnvelope
    }),
)

metrics := pkg.NewEnvelope(
    pkg.WithId(2),
    pkg.WithType("metrics"),
    pkg.WithInterval(3*time.Second),
    pkg.WithDeadline(1*time.Second),
    pkg.WithInvoke(func(ctx context.Context) error {
        fmt.Println("metrics tick", time.Now())
        return nil
    }),
)

q.Start(ctx)
_ = q.Add(email, metrics)

time.AfterFunc(25*time.Second, cancel)
<-ctx.Done()

q.Stop()
```

---

## Поведение очереди

| Сценарий                                                         | Действие очереди                                                                 |
|------------------------------------------------------------------|----------------------------------------------------------------------------------|
| `invoke` вернул `nil`                                            | `Forget`; если `interval > 0` → `AddAfter(interval)`                             |
| Контекст задачи истёк/отменён (`DeadlineExceeded`/`Canceled`)    | `Forget`; если периодическая → `AddAfter(interval)`                              |
| `ErrStopEnvelope` (из `beforeHook`/`invoke`/`afterHook`)         | `Forget` + поместить `_type` в **blacklist**                                     |
| Ошибка в `beforeHook` (не `ErrStopEnvelope`)                     | Периодические: `AddRateLimited`; одноразовые: `Forget`                           |
| Ошибка в `invoke` (не `ErrStopEnvelope`)                         | Периодические: `AddRateLimited`; одноразовые: `Forget`                           |
| Ошибка в `afterHook` (не `ErrStopEnvelope`)                      | Возвращается наверх → те же правила, что и для обычной ошибки                   |
| Паника внутри обработки элемента                                 | Элемент `Forget+Done`, стек логируется; воркер продолжает работу                 |

> Валидация: для периодических задач `deadline` **не должен превышать** `interval` — иначе `ErrAdditionEnvelopeToQueueBadIntervals`.

---

## Stamps (middleware)

Stamps — это обёртки вокруг `Invoker` (обработчика конверта).

- **Глобальные stamps** — задаются на очередь через `WithStamps(...)`.
- **Per-envelope stamps** — через `WithStampsPerEnvelope(...)` в `NewEnvelope(...)`.

Порядок: глобальные идут **первее** и становятся **внешними** (самыми «оборачивающими»), затем per-envelope — **внутренние**.

### Встроенные stamps

- `BeforeAfterStamp(withTimeout)` — исполняет `beforeHook` и `afterHook` с отдельными тайм-бюджетами; любые ошибки, кроме `ErrStopEnvelope`, **возвращаются** наверх. Рекомендуемая функция тайм-бюджета:  
  `WithHookTimeout(ctx, base=deadline, frac=0.5, min=800ms)` → `max(50% от deadline, 800ms)`.
- `LoggingStamp(l *log.Logger)` — логирует длительность и ошибку обработки конверта.

---

## Публичные функции и опции

### Конструктор и геттеры

```go
e := pkg.NewEnvelope(opts...)

id   := e.GetId()
name := e.GetType()
st   := e.GetStamps()
```

### Опции `Envelope`

```go
pkg.WithId(id uint64)
pkg.WithType(t string)
pkg.WithInterval(d time.Duration)      // 0 = одноразовая задача
pkg.WithDeadline(d time.Duration)      // 0 = без таймаута
pkg.WithBeforeHook(func(ctx context.Context, e *Envelope) error)
pkg.WithInvoke(func(ctx context.Context) error)
pkg.WithAfterHook(func(ctx context.Context, e *Envelope) error)
pkg.WithStampsPerEnvelope(stamps ...Stamp)
```

### Опции очереди

```go
pkg.WithLimitOption(n)                        // число воркеров (>0)
pkg.WithWaitingOption(true|false)             // ждать ли завершения воркеров в Stop()
pkg.WithStopModeOption(pkg.Drain|pkg.Stop)
pkg.WithLimiterOption(customLimiter)          // если не задан — дефолтный
pkg.WithWorkqueueConfigOption(conf)           // конфиг workqueue
pkg.WithStamps(stamps...)                     // глобальные stamps
```

### Ошибки

```go
pkg.ErrStopEnvelope                         // поместить `_type` в blacklist
pkg.ErrEnvelopeInBlacklist                  // попытка добавить тип из blacklist
pkg.ErrEnvelopeQueueIsNotRunning            // Add до Start/после Stop
pkg.ErrAdditionEnvelopeToQueueBadFields     // пустой тип / nil invoke / отрицательные интервалы
pkg.ErrAdditionEnvelopeToQueueBadIntervals  // deadline > interval для периодических
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