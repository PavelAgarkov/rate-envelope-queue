package pkg

import (
	"context"
	"errors"
	"log"
	"time"
)

type Envelope struct {
	Id       uint64
	Type     string
	Interval time.Duration
	Deadline time.Duration

	BeforeHook func(ctx context.Context, item *Envelope) error
	Invoke     func(ctx context.Context) error
	AfterHook  func(ctx context.Context, item *Envelope) error

	Stamps []Stamp // per-envelope stamps
}

func WithHookTimeout(ctx context.Context, base time.Duration, frac float64, min time.Duration) (context.Context, context.CancelFunc) {
	d := time.Duration(float64(base) * frac)
	if d < min {
		d = min
	}
	if base == 0 {
		d = min
	}
	return context.WithTimeout(ctx, d)
}

func WithStamps(stamps ...Stamp) func(*RateEnvelopeQueue) {
	return func(q *RateEnvelopeQueue) {
		q.queueStamps = append(q.queueStamps, stamps...)
	}
}

func BeforeAfterStamp(withTimeout func(ctx context.Context, base time.Duration, frac float64, min time.Duration) (context.Context, context.CancelFunc)) Stamp {
	return func(next Invoker) Invoker {
		return func(ctx context.Context, envelope *Envelope) error {
			if envelope.BeforeHook != nil {
				hctx, cancel := withTimeout(ctx, envelope.Deadline, 0.2, 800*time.Millisecond)
				err := envelope.BeforeHook(hctx, envelope)
				cancel()
				if err != nil {
					if errors.Is(err, ErrStopEnvelope) {
						return ErrStopEnvelope
					}
					return err
				}
			}

			// основной вызов
			err := next(ctx, envelope)

			if envelope.AfterHook != nil {
				hctx, cancel := withTimeout(ctx, envelope.Deadline, 0.2, 800*time.Millisecond)
				aerr := envelope.AfterHook(hctx, envelope)
				cancel()

				if aerr != nil {
					if errors.Is(aerr, ErrStopEnvelope) {
						return ErrStopEnvelope
					}
					//log.Printf("%s: envelope %s/%d after hook error: %v", service, envelope.Type, envelope.Id, aerr)
					return aerr
				}
			}
			return err
		}
	}
}

func LoggingStamp(l *log.Logger) Stamp {
	return func(next Invoker) Invoker {
		return func(ctx context.Context, envelope *Envelope) error {
			t0 := time.Now()
			err := next(ctx, envelope)
			l.Printf("%s %s/%d dur=%s err=%v", service, envelope.Type, envelope.Id, time.Since(t0), err)
			return err
		}
	}
}
