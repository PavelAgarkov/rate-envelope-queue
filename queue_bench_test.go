package rate_envelope_queue

import (
	"context"
	"testing"
	"time"
)

// go test -bench=BenchmarkQueueFull -benchmem
func BenchmarkQueueFull(b *testing.B) {
	q := NewRateEnvelopeQueue(context.Background(), "bench",
		WithLimitOption(64),                  // количество воркеров
		WithStopModeOption(Drain),            // корректный останов
		WithAllowedCapacityOption(9_000_000), // высокая ёмкость
	)
	q.Start()
	defer q.Stop()

	// пустые хуки
	before := func(ctx context.Context, e *Envelope) error { return nil }
	invoke := func(ctx context.Context, e *Envelope) error { return nil }
	after := func(ctx context.Context, e *Envelope) error { return nil }
	failure := func(ctx context.Context, e *Envelope, err error) Decision {
		return DefaultOnceDecision()
	}
	success := func(ctx context.Context, e *Envelope) {}

	env, _ := NewEnvelope(
		WithId(1),
		WithType("bench"),
		WithBeforeHook(before),
		WithInvoke(invoke),
		WithAfterHook(after),
		WithFailureHook(failure),
		WithSuccessHook(success),
		WithDeadline(50*time.Millisecond),
	)

	// сбрасываем таймер, чтобы не учитывать сетап
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := q.Send(env); err != nil {
			b.Fatal(err)
		}
	}
}

// go test -bench=BenchmarkQueueInterval -benchmem
func BenchmarkQueueInterval(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	q := NewRateEnvelopeQueue(ctx, "bench-interval",
		WithLimitOption(64),                  // воркеры
		WithStopModeOption(Drain),            // аккуратный останов
		WithAllowedCapacityOption(1_000_000), // большой запас
	)
	q.Start()
	defer q.Stop()

	before := func(ctx context.Context, e *Envelope) error { return nil }
	invoke := func(ctx context.Context, e *Envelope) error { return nil }
	after := func(ctx context.Context, e *Envelope) error { return nil }
	failure := func(ctx context.Context, e *Envelope, err error) Decision {
		return DefaultOnceDecision()
	}
	success := func(ctx context.Context, e *Envelope) {}

	// периодическая задача с небольшим интервалом
	env, _ := NewEnvelope(
		WithId(1),
		WithType("interval"),
		WithBeforeHook(before),
		WithInvoke(invoke),
		WithAfterHook(after),
		WithFailureHook(failure),
		WithSuccessHook(success),
		WithScheduleModeInterval(30*time.Millisecond), // каждые 30мс
		WithDeadline(20*time.Millisecond),
	)

	// загружаем одну задачу для перепланировки
	if err := q.Send(env); err != nil {
		b.Fatal(err)
	}

	// сбрасываем таймер, чтобы не учитывать сетап
	b.ResetTimer()

	// прогоняем b.N итераций, чтобы нагрузить планировщик AddAfter
	for i := 0; i < b.N; i++ {
		// динамически создаём ещё один Envelope, чтобы система обрабатывала их параллельно
		e, _ := NewEnvelope(
			WithId(uint64(i+2)),
			WithType("dyn"),
			WithBeforeHook(before),
			WithInvoke(invoke),
			WithAfterHook(after),
			WithFailureHook(failure),
			WithSuccessHook(success),
			WithScheduleModeInterval(time.Millisecond*11), // короче интервал
			WithDeadline(time.Millisecond*10),
		)
		if err := q.Send(e); err != nil {
			b.Fatal(err)
		}
	}
}

/*
Benchmark results (user-reported)

$ go test -bench=BenchmarkQueueFull -benchmem
  4874815               315.0 ns/op            18 B/op          1 allocs/op
PASS
ok      github.com/PavelAgarkov/rate-envelope-queue     1.796s

$ go test -bench=BenchmarkQueueInterval -benchmem
    97928             13335 ns/op            1715 B/op         22 allocs/op
PASS
ok      github.com/PavelAgarkov/rate-envelope-queue     2.297s
*/
