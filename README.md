# rate-envelope-queue

Лёгкий пакет для управления пулом задач (**envelopes**) поверх `k8s.io/client-go/util/workqueue` с ограничением параллелизма, ретраями, периодическим планированием и хуками до/после выполнения.

> Основано на `workqueue` из client-go: очередь дедуплицирует одинаковые элементы (один и тот же **указатель**) и поддерживает rate-limiting / отложенное перепланирование.

---

## Возможности

- **Фиксированный пул воркеров**: настраиваемый параллелизм через `WithLimitOption`.
- **Периодические и одноразовые задачи**: `Interval > 0` → периодические, `Interval == 0` → одноразовые.
- **Дедлайны**: `Deadline > 0` ограничивает время выполнения `Invoke`.
- **Хуки**: `BeforeHook`/`AfterHook` с отдельным тайм‑бюджетом.
- **Сентинель для остановки типа**: верните `ErrStopEnvelope` из `BeforeHook`/`Invoke`/`AfterHook`, чтобы добавить `Type` в blacklist и прекратить дальнейшее планирование таких задач.
- **Грациозная остановка**: `Drain` (дождаться завершения) или `Stop` (остановиться сразу).
- **Динамическое добавление** задач во время работы.
- **Явная валидация** входных данных и понятные ошибки.

---

## Требования

- Go **1.22+** (используется `for range <int>`).
- Модули:
    - `k8s.io/client-go/util/workqueue`
    - `golang.org/x/time/rate`

---

## Установка

```bash
go get github.com/PavelAgarkov/rate-pool/pkg
```

Импорт:

```go
import "github.com/PavelAgarkov/rate-pool/pkg"
```

---

## Быстрый старт

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/PavelAgarkov/rate-pool/pkg"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Очередь: 3 воркера, дожидаемся drain при остановке
	q := pkg.NewRateEnvelopeQueue(
		pkg.WithLimitOption(3),
		pkg.WithWaitingOption(true),
		pkg.WithStopModeOption(pkg.Drain),
		// лимитер/конфиг можно не задавать — будет дефолт
	)

	// Периодическая задача (каждые 3с) с дедлайном 1с
	metrics := &pkg.Envelope{
		Id:       2,
		Type:     "metrics",
		Interval: 3 * time.Second,
		Deadline: 1 * time.Second,
		Invoke: func(ctx context.Context) error {
			fmt.Println("📊 metrics tick", time.Now())
			return nil
		},
	}

	// Одноразовая задача (Interval == 0)
	oneShot := &pkg.Envelope{
		Id:       3,
		Type:     "oneshot",
		Interval: 0,
		Deadline: 2 * time.Second,
		Invoke: func(ctx context.Context) error {
			fmt.Println("🔥 single run", time.Now())
			return nil
		},
	}

	q.Start(ctx)
	if err := q.Add(metrics, oneShot); err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Second)
	q.Stop() // Drain: дождётся завершения текущих работ
}
```

---

## Поведение

| Сценарий                                         | Действие очереди                                                       |
|--------------------------------------------------|------------------------------------------------------------------------|
| `Invoke` вернул `nil`                            | `Forget` + для периодических: `AddAfter(Interval)`                     |
| Контекст истёк / отменён                         | `Forget` + для периодических: `AddAfter(Interval)`                     |
| `Invoke` вернул ошибку (не `ErrStopEnvelope`)    | Периодические: `AddRateLimited` (бэкофф); одноразовые: `Forget`        |
| Возврат `ErrStopEnvelope` (любой хук/Invoke)     | `Forget` + поместить `Type` в **blacklist**                            |
| Ошибка в `BeforeHook` (не `ErrStopEnvelope`)     | Периодические: `AddRateLimited`; одноразовые: `Forget`                 |
| Ошибка в `AfterHook` (не `ErrStopEnvelope`)      | Только лог; если `ErrStopEnvelope` — тип в blacklist                   |

> `Deadline == 0` → **без таймаута**. Для периодических задач рекомендуется `Deadline <= Interval` (валидация это проверяет).

---

## Статический vs динамический набор

- **Статический набор**: поддерживайте массив указателей на `Envelope` и добавляйте его один раз — периодические будут перепланироваться, пока вы не остановите очередь или не вернёте `ErrStopEnvelope`.
- **Динамический набор**: добавляйте новые `Envelope` в любой момент через `Add(...)`.
- **Важно**: дедупликация в `workqueue` работает по **компарабельности элемента**; для указателей — по адресу. Для корректной дедупликации **не меняйте указатель** на `Envelope` во время жизни элемента в очереди.

---

## API

### Типы

```go
type StopMode string

const (
	Drain StopMode = "drain" // дождаться обработки (graceful)
	Stop  StopMode = "stop"  // остановиться сразу
)

type Envelope struct {
	Id       uint64
	Type     string
	Interval time.Duration // 0 = одноразовая задача
	Deadline time.Duration // 0 = без таймаута

	BeforeHook func(ctx context.Context, item *Envelope) error
	Invoke     func(ctx context.Context) error
	AfterHook  func(ctx context.Context, item *Envelope) error
}

type QueuePool interface {
	Start(ctx context.Context)
	Add(envelopes ...*Envelope) error
	Stop()
}
```

### Создание очереди

```go
q := pkg.NewRateEnvelopeQueue(
	pkg.WithLimitOption(n),                 // число воркеров (>0)
	pkg.WithWaitingOption(true|false),      // ждать ли завершения воркеров в Stop()
	pkg.WithStopModeOption(pkg.Drain|pkg.Stop),
	pkg.WithLimiterOption(customLimiter),   // опционально; если nil — дефолт
	pkg.WithWorkqueueConfigOption(conf),    // клиентский конфиг очереди (например, Name для метрик)
)
```

**Дефолтный rate-limiter**:  
`MaxOf(ItemExponentialFailureRateLimiter(1s..30s), BucketRateLimiter(5 rps, burst=10))`.

### Ошибки

- `ErrStopEnvelope` — положить `Type` в blacklist (верните её из `BeforeHook`/`Invoke`/`AfterHook`).
- `ErrEnvelopeInBlacklist` — попытка добавить envelope с типом из blacklist.
- `ErrEnvelopeQueueIsNotRunning` — вызов `Add` до `Start`/после `Stop`.
- `ErrAdditionEnvelopeToQueueBadFields` — `Type == ""`, `Invoke == nil`, `Interval < 0`, `Deadline < 0`.
- `ErrAdditionEnvelopeToQueueBadIntervals` — для периодических `Deadline > Interval`.

---

## Пример из теста (упрощённый)

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

email := &pkg.Envelope{
	Id:       1,
	Type:     "email",
	Interval: 5 * time.Second,
	Deadline: 3 * time.Second,
	Invoke: func(ctx context.Context) error {
		time.Sleep(5 * time.Second) // превысит дедлайн
		fmt.Println("📧 Email v1", time.Now())
		return nil
	},
	BeforeHook: func(ctx context.Context, e *pkg.Envelope) error {
		fmt.Println("hook before email", e.Id, time.Now())
		return nil
	},
	AfterHook: func(ctx context.Context, e *pkg.Envelope) error {
		fmt.Println("hook after email", e.Id, time.Now())
		// Остановим дальнейшие email
		return pkg.ErrStopEnvelope
	},
}

metrics := &pkg.Envelope{
	Id:       2,
	Type:     "metrics",
	Interval: 3 * time.Second,
	Deadline: 1 * time.Second,
	Invoke: func(ctx context.Context) error {
		fmt.Println("📊 Metrics", time.Now())
		return nil
	},
}

food := &pkg.Envelope{
	Id:       3,
	Type:     "food",
	Interval: 2 * time.Second,
	Deadline: 1 * time.Second,
	Invoke: func(ctx context.Context) error {
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
_ = q.Add(email, metrics, food, email) // второй add(email) будет дедуплицирован

// по таймеру завершить приложение
time.AfterFunc(25*time.Second, cancel)
<-ctx.Done()

q.Stop()
fmt.Println("queue: done")
```

---

## Замечания по эксплуатации

- Чтобы одинаковые периодические задачи не «стреляли строем», можно добавить **jitter** к `AddAfter` (±5–10%).
- В `workqueue` есть имя очереди (через конфиг), это удобно для экспонирования **метрик**.
- Очередь можно остановить в любой момент (`Stop()`); для повторного использования создайте **новый объект** очереди.

---

## Лицензия

MIT

---

**Префикс логов:** `"[rate-envelope-queue]"`. 