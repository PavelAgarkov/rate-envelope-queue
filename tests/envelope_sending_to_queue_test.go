package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	req "github.com/PavelAgarkov/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
)

func TestEnvelopeAddingToQueue(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 20*time.Second)

	t.Run("Single envelope adding to queue", func(t *testing.T) {
		isCalledInInvokeBeforeStart := false
		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvokeBeforeStart = true
				return nil
			}),
		)
		assert.NoError(t, err)

		isCalledInInvokeAfterStart := false
		envelope2, err := req.NewEnvelope(
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
			err = envelopeQueue.Send(envelope2)
			assert.NoError(t, err)
			envelopeQueue.Start()

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, true, isCalledInInvokeAfterStart)
			}

			envelopeQueue.Stop()

			envelopeQueue.Start()
			err = envelopeQueue.Send(envelope)
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
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithScheduleModeInterval(time.Duration(invalidInterval)),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)
		assert.NoError(t, err)

		invalidDeadlineEnvelope, err := req.NewEnvelope(
			req.WithId(3),
			req.WithType("envelope_3"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(time.Duration(invalidDeadline)),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)
		assert.NoError(t, err)

		deadlineGreaterThanIntervalEnvelope, err := req.NewEnvelope(
			req.WithId(4),
			req.WithType("envelope_4"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(3*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)

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

			err = envelopeQueue.Send(deadlineGreaterThanIntervalEnvelope)
			assert.ErrorIs(t, err, req.ErrAdditionEnvelopeToQueueBadIntervals)

		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Adding to queue with not running queue", func(t *testing.T) {
		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
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

		test := func() {
			envelopeQueue.Start()
			envelopeQueue.Stop()

			err = envelopeQueue.Send(envelope)
			assert.ErrorIs(t, err, req.ErrEnvelopeQueueIsNotRunning)
		}

		test()
	})

	t.Run("Adding to queue with not queue init and queue pending option", func(t *testing.T) {
		invokeCallCh := make(chan struct{}, 3)

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				invokeCallCh <- struct{}{}
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

		test := func() {
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			for i := 2; i < 4; i++ {
				envelope_i, err := req.NewEnvelope(
					req.WithId(uint64(i)),
					req.WithType(fmt.Sprintf("envelope_%d", i)),
					req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
						invokeCallCh <- struct{}{}
						return nil
					}),
				)
				assert.NoError(t, err)

				err = envelopeQueue.Send(envelope_i)
				assert.NoError(t, err)
			}

			envelopeQueue.Start()

			select {
			case <-time.After(1 * time.Second):
				close(invokeCallCh)
			}

			assert.Equal(t, 3, len(invokeCallCh))

			envelopeQueue.Stop()
		}

		test()
	})

	t.Run("Adding to queue with queue stopping in another goroutine", func(t *testing.T) {
		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)
		assert.NoError(t, err)

		envelope2, err := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope"),
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

		test := func() {
			var mu sync.Mutex
			cond := sync.NewCond(&mu)
			var readyCount int

			envelopeQueue.Start()

			go func() {
				mu.Lock()
				readyCount++
				cond.Broadcast()
				cond.Wait()
				mu.Unlock()

				envelopeQueue.Stop()
				err = envelopeQueue.Send(envelope)
				assert.ErrorIs(t, err, req.ErrEnvelopeQueueIsNotRunning)
			}()

			go func() {
				mu.Lock()
				readyCount++
				cond.Broadcast()
				cond.Wait()
				mu.Unlock()

				time.Sleep(10 * time.Millisecond)
				err = envelopeQueue.Send(envelope2)
				assert.ErrorIs(t, err, req.ErrEnvelopeQueueIsNotRunning)
			}()

			mu.Lock()
			for readyCount < 2 {
				cond.Wait()
			}

			cond.Broadcast()
			mu.Unlock()

			time.Sleep(100 * time.Millisecond)
		}

		test()
	})

	t.Run("Adding to queue same envelope pointer multiple times", func(t *testing.T) {
		var invokeCallCounter int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeCallCounter++
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

			err = envelopeQueue.Send(envelope, envelope, envelope)
			assert.NoError(t, err)
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(4 * time.Second):
				assert.Equal(t, 1, invokeCallCounter)
			}
		}

		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Adding to queue with stopped queue after terminate option", func(t *testing.T) {
		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
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

		test := func() {
			envelopeQueue.Start()
			envelopeQueue.Stop()
			envelopeQueue.Terminate()

			err = envelopeQueue.Send(envelope)
			assert.ErrorIs(t, err, req.ErrQueueIsTerminated)
		}

		test()
	})
}
