package tests

import (
	"context"
	"testing"
	"time"

	req "github.com/PavelAgarkov/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
)

func TestEnvelopeAddingToQueue(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

	t.Run("Single envelope adding to queue", func(t *testing.T) {
		isCalledInInvokeBeforeStart := false
		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvokeBeforeStart = true
				return nil
			}),
		)
		assert.NoError(t, err)

		isCalledInInvokeAfterStart := false
		someEnvelope2, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvokeAfterStart = true
				return nil
			}),
		)
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"some queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			err = envelopeQueue.Send(someEnvelope2)
			assert.NoError(t, err)
			envelopeQueue.Start()

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, true, isCalledInInvokeAfterStart)
			}

			envelopeQueue.Stop()

			envelopeQueue.Start()
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, true, isCalledInInvokeBeforeStart)
			}

			envelopeQueue.Stop()
		}

		start()
	})

	t.Run("Multiple envelope adding to queue", func(t *testing.T) {
		targetCallsOrder := []string{"first", "second"}
		var actualCallsOrder []string

		firstEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCallsOrder = append(actualCallsOrder, "first")
				return nil
			}),
		)
		assert.NoError(t, err)

		secondEnvelope, err2 := req.NewEnvelope(
			req.WithId(2),
			req.WithType("secondEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCallsOrder = append(actualCallsOrder, "second")
				return nil
			}),
		)
		assert.NoError(t, err2)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(firstEnvelope, secondEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, targetCallsOrder, actualCallsOrder)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	// negative
	t.Run("Adding to queue with invalid parameters", func(t *testing.T) {
		invalidInterval := -1
		invalidDeadline := -1
		var nilInvoke func(ctx context.Context, envelope *req.Envelope) error

		nilInvokeEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(nilInvoke),
		)
		assert.Error(t, err)

		invalidIntervalEnvelope, err := req.NewEnvelope(
			req.WithScheduleModeInterval(time.Duration(invalidInterval)),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)
		assert.NoError(t, err)

		invalidDeadlineEnvelope, err := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(time.Duration(invalidDeadline)),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
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

			err = envelopeQueue.Send(nilInvokeEnvelope)
			assert.ErrorIs(t, err, req.ErrAdditionEnvelopeToQueueBadFields)

			err = envelopeQueue.Send(invalidIntervalEnvelope)
			assert.ErrorIs(t, err, req.ErrAdditionEnvelopeToQueueBadFields)

			err = envelopeQueue.Send(invalidDeadlineEnvelope)
			assert.ErrorIs(t, err, req.ErrAdditionEnvelopeToQueueBadFields)

		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Adding to queue with not running queue", func(t *testing.T) {
		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
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
			envelopeQueue.Stop()

			err = envelopeQueue.Send(someEnvelope)
			assert.Error(t, err)
			assert.ErrorIs(t, err, req.ErrEnvelopeQueueIsNotRunning)

			envelopeQueue.Start()
		}

		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	// negative
	t.Run("Envelope adding to queue with mismatch timeouts", func(t *testing.T) {
		firstEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(3*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
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
			err = envelopeQueue.Send(firstEnvelope)
			assert.ErrorIs(t, req.ErrAdditionEnvelopeToQueueBadIntervals, err)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

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
}
