package tests

import (
	"context"
	"fmt"
	req "github.com/simplegear/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestBaseWorkerBehaviourOnError(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 10*time.Second)

	t.Run("Worker error in beforehook without afterhook", func(t *testing.T) {
		isCalledInInvoke := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("error")
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvoke = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, false, isCalledInInvoke)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker error in beforehook with skip afterhook", func(t *testing.T) {
		isCalledInInvoke := false
		isCalledInAfterHook := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("error")
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvoke = true
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInAfterHook = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, false, isCalledInInvoke)
				assert.Equal(t, false, isCalledInAfterHook)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker error in beforehook with success call afterhook", func(t *testing.T) {
		isCalledInInvoke := false
		isCalledInAfterHook := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return req.ErrStopEnvelope
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvoke = true
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInAfterHook = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, false, isCalledInInvoke)
				assert.Equal(t, true, isCalledInAfterHook)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker error in invoke with skip afterhook", func(t *testing.T) {
		isCalledInAfterHook := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("error")
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInAfterHook = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, false, isCalledInAfterHook)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker error in invoke with success call afterhook", func(t *testing.T) {
		isCalledInAfterHook := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return req.ErrStopEnvelope
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInAfterHook = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, true, isCalledInAfterHook)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}

func TestWorkerCtxCancelledOrDeadlineExceededOnPeriodicEnvelope(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 20*time.Second)

	t.Run("Worker beforehook ctx canceled with default deadline value", func(t *testing.T) {
		targetCycleCall := 2
		errorCycleCall := 1

		actualCycle := 0

		var cycleCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				actualCycle++

				switch actualCycle {
				case errorCycleCall:
					select {
					case <-ctx.Done(): // default deadline parameter=2000ms
						return ctx.Err()
					}
				case targetCycleCall:
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualCycle {
				case errorCycleCall:
					cycleCalls = append(cycleCalls, actualCycle)
				case targetCycleCall:
					cycleCalls = append(cycleCalls, actualCycle)
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(4 * time.Second):
				assert.Equal(t, []int{2}, cycleCalls)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker beforehook context canceled with queue stop", func(t *testing.T) {
		unworkedCycleCall := 2
		contextCancelCycle := 1

		actualCycle := 0
		var cycleCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				actualCycle++

				switch contextCancelCycle {
				case contextCancelCycle:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				case unworkedCycleCall:
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualCycle {
				case contextCancelCycle:
					cycleCalls = append(cycleCalls, actualCycle)
				case unworkedCycleCall:
					cycleCalls = append(cycleCalls, actualCycle)
				}
				return nil
			}),
		)
		assert.NoError(t, err)

		ctx, cancel := context.WithCancel(suite.ctx)
		envelopeQueue := req.NewRateEnvelopeQueue(
			ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			ctxCancelTimer := time.NewTimer(100 * time.Millisecond)
			<-ctxCancelTimer.C
			cancel()

			assertTimer := time.NewTimer(2 * time.Second)
			<-assertTimer.C
			assert.Equal(t, 0, len(cycleCalls))
			assert.Equal(t, 1, actualCycle)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker invoke context canceled with queue stop", func(t *testing.T) {
		unworkedCycleCall := 2
		contextCancelCycle := 1

		actualCycle := 0
		var cycleCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				actualCycle++

				switch actualCycle {
				case contextCancelCycle:
					cycleCalls = append(cycleCalls, actualCycle)
				case unworkedCycleCall:
					cycleCalls = append(cycleCalls, actualCycle)
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualCycle {
				case 1:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				case unworkedCycleCall:
				}
				return nil
			}),
		)
		assert.NoError(t, err)

		ctx, cancel := context.WithCancel(suite.ctx)
		envelopeQueue := req.NewRateEnvelopeQueue(
			ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			ctxCancelTimer := time.NewTimer(100 * time.Millisecond)
			<-ctxCancelTimer.C
			cancel()

			assertTimer := time.NewTimer(2 * time.Second)
			<-assertTimer.C
			assert.Equal(t, []int{contextCancelCycle}, cycleCalls)
			assert.Equal(t, 1, actualCycle)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker afterhook context canceled with queue stop", func(t *testing.T) {
		unworkedCycleCall := 2
		contextCancelCycle := 1

		actualCycle := 0
		var cycleCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				actualCycle++

				switch actualCycle {
				case contextCancelCycle:
					cycleCalls = append(cycleCalls, actualCycle)
				case unworkedCycleCall:
					cycleCalls = append(cycleCalls, actualCycle)
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualCycle {
				case 1:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				case unworkedCycleCall:
				}
				return nil
			}),
		)
		assert.NoError(t, err)

		ctx, cancel := context.WithCancel(suite.ctx)
		envelopeQueue := req.NewRateEnvelopeQueue(
			ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			ctxCancelTimer := time.NewTimer(100 * time.Millisecond)
			<-ctxCancelTimer.C
			cancel()

			assertTimer := time.NewTimer(2 * time.Second)
			<-assertTimer.C
			assert.Equal(t, []int{1}, cycleCalls)
			assert.Equal(t, 1, actualCycle)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker afterhook context canceled with skip failurehook", func(t *testing.T) {
		unworkedCycleCall := 2
		contextCancelCycle := 1

		actualCycle := 0
		var cycleCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				actualCycle++

				switch actualCycle {
				case contextCancelCycle:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				case unworkedCycleCall:
				}
				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				cycleCalls = append(cycleCalls, actualCycle)
				return req.DefaultOnceDecision()
			}),
		)
		assert.NoError(t, err)

		ctx, cancel := context.WithCancel(suite.ctx)
		envelopeQueue := req.NewRateEnvelopeQueue(
			ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			ctxCancelTimer := time.NewTimer(100 * time.Millisecond)
			<-ctxCancelTimer.C
			cancel()

			assertTimer := time.NewTimer(2 * time.Second)
			<-assertTimer.C
			assert.Equal(t, 0, len(cycleCalls))
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}

func TestIntervalCallingOnPeriodicEnvelopeWithError(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 10*time.Second)

	t.Run("Envelope rescheduling if error is in beforehook calling", func(t *testing.T) {
		successCycleCall := 2
		errorCycle := 1

		actualCycle := 0
		var cycleCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				actualCycle++

				switch actualCycle {
				case errorCycle:
					return fmt.Errorf("error")
				case successCycleCall:
					return nil
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				cycleCalls = append(cycleCalls, actualCycle)
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1*time.Second + 100*time.Millisecond):
				assert.Equal(t, []int{successCycleCall}, cycleCalls)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope rescheduling if error is in invoke calling", func(t *testing.T) {
		successCycleCall := 2
		errorCycle := 1

		actualCycle := 0
		invokeCallMark := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCycle++

				switch actualCycle {
				case errorCycle:
					return fmt.Errorf("error")
				case successCycleCall:
					invokeCallMark = true
				}
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1*time.Second + 100*time.Millisecond):
				assert.Equal(t, true, invokeCallMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope default interval calling and skip error call if error is in afterhook", func(t *testing.T) {
		cycleCall := 0
		var cycleMarkCallsCounter []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				cycleCall++
				cycleMarkCallsCounter = append(cycleMarkCallsCounter, cycleCall)

				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("error")
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1*time.Second + 100*time.Millisecond): // ожидаем 2 цикла выполнения envelope
				assert.Equal(t, []int{1, 2}, cycleMarkCallsCounter)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}

func TestFailureHookDecisionsOnOneTimeEnvelope(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 1*time.Minute)

	t.Run("Envelope automatic drop decision setting if WithFailureHook is nil", func(t *testing.T) {
		successCycleCall := 2
		errorCycle := 1

		cycleCall := 0
		var successHookCycleCall []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				cycleCall++

				switch cycleCall {
				case errorCycle:
					return fmt.Errorf("error")
				case successCycleCall:
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return nil
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookCycleCall = append(successHookCycleCall, cycleCall)
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(2 * time.Second):
				assert.Equal(t, 1, cycleCall)
				assert.Equal(t, 0, len(successHookCycleCall))
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope WithFailureHook DefaultOnceDecision", func(t *testing.T) {
		successCycleCall := 2
		errorCycle := 1

		cycleCall := 0

		successHookMark := false
		var successHookCycleCall []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				cycleCall++

				switch cycleCall {
				case errorCycle:
					return fmt.Errorf("error")
				case successCycleCall:
					successHookCycleCall = append(successHookCycleCall, cycleCall)
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.DefaultOnceDecision()
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, 0, len(successHookCycleCall))
				assert.Equal(t, false, successHookMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope WithFailureHook RetryOnceNowDecision", func(t *testing.T) {
		successCycleCall := 2
		errorCycle := 1

		cycleCall := 0

		successHookMark := false
		var successHookCycleCall []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				time.Sleep(500 * time.Millisecond)
				cycleCall++

				switch cycleCall {
				case errorCycle:

					return fmt.Errorf("error")
				case successCycleCall:
					successHookCycleCall = append(successHookCycleCall, cycleCall)
				case successCycleCall + 1:
					successHookCycleCall = append(successHookCycleCall, cycleCall)
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceNowDecision()
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			// первый вызов внутри invoke с ошибкой
			select {
			case <-time.After(750 * time.Millisecond):
				assert.Equal(t, 0, len(successHookCycleCall))
				assert.Equal(t, false, successHookMark)
			}

			// второй вызов внутри invoke (уже успешный)
			select {
			case <-time.After(750 * time.Millisecond):
				assert.Equal(t, []int{successCycleCall}, successHookCycleCall)
				assert.Equal(t, true, successHookMark)
			}

			// нового вызова нет, после повторного вызова в invoke envelope забыт через Forget
			select {
			case <-time.After(750 * time.Millisecond):
				assert.Equal(t, []int{successCycleCall}, successHookCycleCall)
				assert.Equal(t, successCycleCall, cycleCall)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope RetryOnceAfterDecision with deadline timeout", func(t *testing.T) {
		successCycleCall := 2
		errorCycle := 1

		cycleCall := 0

		successHookMark := false
		var successHookCycleCall []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				cycleCall++

				switch cycleCall {
				case errorCycle:

					return fmt.Errorf("error")
				case successCycleCall:
					successHookCycleCall = append(successHookCycleCall, cycleCall)
				case successCycleCall + 1:
					successHookCycleCall = append(successHookCycleCall, cycleCall)
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(2 * time.Second)
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			// первый вызов invoke
			select {
			case <-time.After(500 * time.Millisecond):
				assert.Equal(t, 0, len(successHookCycleCall))
				assert.Equal(t, false, successHookMark)
			}

			// invoke отложен с дедлайном в WithFailureHook, isCalledInRetryCount тот же
			select {
			case <-time.After(500 * time.Millisecond):
				assert.Equal(t, 0, len(successHookCycleCall))
				assert.Equal(t, false, successHookMark)
			}

			// дедлайн из WithFailureHook прошел, новый вызов invoke
			select {
			case <-time.After(2 * time.Second):
				assert.Equal(t, []int{successCycleCall}, successHookCycleCall)
				assert.Equal(t, true, successHookMark)
			}

			// нового вызова нет, после повторного вызова в invoke envelope забыт через Forget
			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, []int{successCycleCall}, successHookCycleCall)
				assert.Equal(t, successCycleCall, cycleCall)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope automatic deadline setting if RetryOnceAfterDecision timeout is 0", func(t *testing.T) {
		successCycleCall := 2
		errorCycle := 1

		cycleCall := 0

		successHookMark := false
		var successHookCycleCall []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				cycleCall++

				switch cycleCall {
				case errorCycle:
					return fmt.Errorf("error")
				case successCycleCall:
					successHookCycleCall = append(successHookCycleCall, cycleCall)
				case successCycleCall + 1:
					successHookCycleCall = append(successHookCycleCall, cycleCall)
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(0) // default = 1s
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			// первый вызов invoke
			select {
			case <-time.After(100 * time.Millisecond):
				assert.Equal(t, 0, len(successHookCycleCall))
				assert.Equal(t, false, successHookMark)
			}

			// invoke отложен с дедлайном в WithFailureHook, isCalledInRetryCount тот же
			select {
			case <-time.After(100 * time.Millisecond):
				assert.Equal(t, 0, len(successHookCycleCall))
				assert.Equal(t, false, successHookMark)
			}

			// дедлайн из WithFailureHook прошел, новый вызов invoke
			select {
			case <-time.After(900 * time.Millisecond):
				assert.Equal(t, []int{successCycleCall}, successHookCycleCall)
				assert.Equal(t, true, successHookMark)
			}

			// нового вызова нет, после повторного вызова в invoke envelope забыт через Forget
			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, []int{successCycleCall}, successHookCycleCall)
				assert.Equal(t, successCycleCall, cycleCall)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Multiple envelope with different timeouts in WithFailureHook in queue win single(1) worker", func(t *testing.T) {
		targetEnvelopeOrderCalls := []string{"first", "third", "fifth", "forth", "second"}
		var actualEnvelopeOrderCalls []string

		firstEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualEnvelopeOrderCalls = append(actualEnvelopeOrderCalls, "first")
				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(1 * time.Second)
			}),
		)
		assert.NoError(t, err)

		currentRetrySecond := 0
		targetRetrySecond := 1
		secondEnvelope, err := req.NewEnvelope(
			req.WithId(2),
			req.WithType("secondEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch currentRetrySecond {
				case 0:
					currentRetrySecond++
					return fmt.Errorf("error")
				case targetRetrySecond:
					actualEnvelopeOrderCalls = append(actualEnvelopeOrderCalls, "second")
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(3 * time.Second)
			}),
		)

		thirdEnvelope, err := req.NewEnvelope(
			req.WithId(3),
			req.WithType("thirdEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualEnvelopeOrderCalls = append(actualEnvelopeOrderCalls, "third")
				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(1 * time.Second)
			}),
		)
		assert.NoError(t, err)

		currentRetryForth := 0
		targetRetryForth := 1
		forthEnvelope, err := req.NewEnvelope(
			req.WithId(4),
			req.WithType("forthEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch currentRetryForth {
				case 0:
					currentRetryForth++
					return fmt.Errorf("error")
				case targetRetryForth:
					actualEnvelopeOrderCalls = append(actualEnvelopeOrderCalls, "forth")
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(2 * time.Second)
			}),
		)
		assert.NoError(t, err)

		failureCallFifth := 1
		fifthEnvelope, err := req.NewEnvelope(
			req.WithId(5),
			req.WithType("fifthEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch failureCallFifth {
				case 1:
					failureCallFifth++
					return fmt.Errorf("error")
				default:
					actualEnvelopeOrderCalls = append(actualEnvelopeOrderCalls, "fifth")
					return nil
				}
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceNowDecision()
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
			err = envelopeQueue.Send(firstEnvelope, secondEnvelope, thirdEnvelope, forthEnvelope, fifthEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(4 * time.Second):
				assert.Equal(t, actualEnvelopeOrderCalls, targetEnvelopeOrderCalls)
			}
		}

		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}

func TestSuccessHookOnOneTimeEnvelope(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 10*time.Second)

	t.Run("Envelope success hook", func(t *testing.T) {
		successHookMark := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, true, successHookMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope success hook is not called when beforehook has error", func(t *testing.T) {
		successHookMark := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("error")
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, false, successHookMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope success hook is not called when invoke has error", func(t *testing.T) {
		successHookMark := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("error")
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, false, successHookMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope success hook ignores error in afterhook", func(t *testing.T) {
		successHookMark := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope"),
			req.WithScheduleModeInterval(0),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("error")
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successHookMark = true
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
			err = envelopeQueue.Send(envelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, true, successHookMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}
