package tests

import (
	"context"
	"testing"
	"time"

	req "github.com/PavelAgarkov/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
)

func TestEnvelope(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 30*time.Second)

	// negative
	t.Run("Envelope creation with empty invoke", func(t *testing.T) {
		_, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithScheduleModeInterval(2*time.Second),
			req.WithDeadline(1*time.Second),
		)
		assert.Error(t, err)
	})

	t.Run("Envelope base interface testing", func(t *testing.T) {
		stamps := []req.Stamp{req.LoggingStamp()}

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithStampsPerEnvelope(
				stamps...,
			),
		)
		assert.NoError(t, err)

		assert.Equal(t, someEnvelope.GetId(), uint64(1))
		assert.Equal(t, someEnvelope.GetType(), "someEnvelope")
		assert.Equal(t, len(someEnvelope.GetStamps()), len(stamps))
	})

	t.Run("Envelope interval calling testing", func(t *testing.T) {
		invokeCounter := make(chan bool, 2)
		defer close(invokeCounter)

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(2*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				invokeCounter <- true
				return nil
			}),
		)
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, 2, len(invokeCounter))
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope interval calling testing with time greater than deadline", func(t *testing.T) {
		invokeCounter := make(chan bool, 2)
		defer close(invokeCounter)

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(2*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(3 * time.Second): // не слушаем контекст дедлайна
					invokeCounter <- true
				}

				return nil
			}),
		)
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(6 * time.Second):
				assert.Equal(t, 1, len(invokeCounter))
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope invoke deadline testing", func(t *testing.T) {
		invokeMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Second):
					invokeMark = true
					return nil
				}
			}),
		)
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, false, invokeMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope hooks deadline testing", func(t *testing.T) {
		var beforeHookIdGetter, afterHookIdGetter uint64
		deadline := 2 * time.Second

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(deadline),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				time.Sleep(deadline * 2) // т.к deadline умножается на frac=0.9
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					beforeHookIdGetter = envelope.GetId()
					return nil
				}
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				time.Sleep(deadline * 2) // т.к deadline умножается на frac=0.9
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					afterHookIdGetter = envelope.GetId()
					return nil
				}
			}),
		)
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(deadline * 3):
				assert.Equal(t, beforeHookIdGetter, uint64(0))
				assert.Equal(t, afterHookIdGetter, uint64(0))
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope hooks deadline min timeout testing", func(t *testing.T) {
		var beforeHookIdGetter, afterHookIdGetter uint64
		envelopeDeadline := 3 * time.Second
		inHookDeadline := 2000 * time.Millisecond // не сработает, т.к min deadline = 2000ms

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(envelopeDeadline),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(inHookDeadline): // не сработает, т.к min deadline = 2000ms
					beforeHookIdGetter = envelope.GetId()
					return nil
				}
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(inHookDeadline): // не сработает, т.к min deadline = 2000ms
					afterHookIdGetter = envelope.GetId()
					return nil
				}
			}),
		)
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, beforeHookIdGetter, uint64(0))
				assert.Equal(t, afterHookIdGetter, uint64(0))
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}
