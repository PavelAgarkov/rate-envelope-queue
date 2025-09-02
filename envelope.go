package rate_envelope_queue

import (
	"context"
	"log"
	"time"
)

type destinationState string

const (
	DestinationStateField                       = "type"
	PayloadAfterField                           = "after"
	DestinationStateRetryNow   destinationState = "retry_now"
	DestinationStataRetryAfter destinationState = "retry_after"
	DestinationStateDrop       destinationState = "drop"
)

type Payload map[string]interface{}

type Decision interface {
	Payload() Payload
}

func NewDefaultDestination() Decision {
	return &DefaultDestination{
		Data: map[string]interface{}{
			DestinationStateField: DestinationStateDrop,
		},
	}
}

type DefaultDestination struct {
	Data map[string]interface{}
}

func (d *DefaultDestination) Payload() Payload {
	return d.Data
}

func NewRetryNowDestination() Decision {
	return &RetryOnceDestination{
		Data: map[string]interface{}{
			DestinationStateField: DestinationStateRetryNow,
		},
	}
}

func NewRetryAfterDestination(after time.Duration) Decision {
	return &RetryOnceDestination{
		Data: map[string]interface{}{
			DestinationStateField: DestinationStataRetryAfter,
			PayloadAfterField:     after,
		},
	}
}

type RetryOnceDestination struct {
	Data map[string]interface{}
}

func (d *RetryOnceDestination) Payload() Payload {
	return d.Data
}

type Envelope struct {
	id    uint64
	_type string

	interval time.Duration
	deadline time.Duration

	// ----- хуки, который разрешают вмешиваться в процесс обработки конверта и остановить его через ошибку ErrStopEnvelope
	beforeHook func(ctx context.Context, envelope *Envelope) error
	invoke     func(ctx context.Context, envelope *Envelope) error
	afterHook  func(ctx context.Context, envelope *Envelope) error
	// -----
	// хук, которй позволяет динамически реагировать по решению пользователя на поведение конверта при ошибке
	failureHook func(ctx context.Context, envelope *Envelope, err error) Decision
	successHook func(ctx context.Context, envelope *Envelope)

	stamps []Stamp // per-envelope stamps
}

func (stamp *Envelope) GetId() uint64 {
	return stamp.id
}

func (stamp *Envelope) GetType() string {
	return stamp._type
}

func (stamp *Envelope) GetStamps() []Stamp {
	return stamp.stamps
}

func WithStampsPerEnvelope(stamps ...Stamp) func(*Envelope) {
	return func(e *Envelope) {
		e.stamps = append(e.stamps, stamps...)
	}
}

func WithBeforeHook(hook Invoker) func(*Envelope) {
	return func(e *Envelope) {
		e.beforeHook = hook
	}
}

func WithAfterHook(hook Invoker) func(*Envelope) {
	return func(e *Envelope) {
		e.afterHook = hook
	}
}

func WithInvoke(invoke Invoker) func(*Envelope) {
	return func(e *Envelope) {
		e.invoke = invoke
	}
}

func WithFailureHook(hook func(ctx context.Context, envelope *Envelope, err error) Decision) func(*Envelope) {
	return func(e *Envelope) {
		e.failureHook = hook
	}
}

func WithSuccessHook(hook func(ctx context.Context, envelope *Envelope)) func(*Envelope) {
	return func(e *Envelope) {
		e.successHook = hook
	}
}

func WithInterval(d time.Duration) func(*Envelope) {
	return func(e *Envelope) {
		e.interval = d
	}
}

func WithDeadline(d time.Duration) func(*Envelope) {
	return func(e *Envelope) {
		e.deadline = d
	}
}

func WithType(t string) func(*Envelope) {
	return func(e *Envelope) {
		e._type = t
	}
}

func WithId(id uint64) func(*Envelope) {
	return func(e *Envelope) {
		e.id = id
	}
}

func NewEnvelope(opt ...func(*Envelope)) *Envelope {
	envelope := &Envelope{}
	for _, o := range opt {
		o(envelope)
	}

	return envelope
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

func LoggingStamp(l *log.Logger) Stamp {
	return func(next Invoker) Invoker {
		return func(ctx context.Context, envelope *Envelope) error {
			t0 := time.Now()
			err := next(ctx, envelope)
			l.Printf("%s %s/%d dur=%s err=%v", service, envelope._type, envelope.id, time.Since(t0), err)
			return err
		}
	}
}
