package tests

import (
	"context"
	"fmt"
	req "github.com/PavelAgarkov/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestBaseWorkerBehaviourOnError(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

	t.Run("Worker error in beforehook without afterhook testing", func(t *testing.T) {
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

	t.Run("Worker error in beforehook with skip afterhook testing", func(t *testing.T) {
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

	t.Run("Worker error in beforehook with success afterhook testing", func(t *testing.T) {
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

	t.Run("Worker error in invoke with skip afterhook testing", func(t *testing.T) {
		isCalledInAfterHook := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
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

	t.Run("Worker error in invoke with skip afterhook testing", func(t *testing.T) {
		isCalledInAfterHook := false

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
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

func TestWorkerCtxCancelledOrDeadlineExceededOnPeriodicEnvelopeError(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

	t.Run("Worker beforehook ctx canceled with default value testing", func(t *testing.T) {
		targetCall := 2
		actualHookCalls := 1
		var invokeCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					select {
					case <-ctx.Done(): // default deadline parameter=800ms
						actualHookCalls++
						return ctx.Err()
					}
				case targetCall:
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					invokeCalls = append(invokeCalls, actualHookCalls)
				case targetCall:
					invokeCalls = append(invokeCalls, actualHookCalls)
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
				assert.Equal(t, []int{2}, invokeCalls)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker beforehook context canceled with queue stop testing", func(t *testing.T) {
		targetCall := 2
		actualHookCalls := 1
		var invokeCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							actualHookCalls++
							return ctx.Err()
						}
					}
				case targetCall:
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					invokeCalls = append(invokeCalls, actualHookCalls)
				case targetCall:
					invokeCalls = append(invokeCalls, actualHookCalls)
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
			assert.Equal(t, 0, len(invokeCalls))
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker invoke context canceled with queue stop testing", func(t *testing.T) {
		targetCall := 2
		actualHookCalls := 1
		var invokeCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					invokeCalls = append(invokeCalls, actualHookCalls)
				case targetCall:
					invokeCalls = append(invokeCalls, actualHookCalls)
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							actualHookCalls++
							return ctx.Err()
						}
					}
				case targetCall:
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
			assert.Equal(t, []int{1}, invokeCalls)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker afterhook context canceled with queue stop testing", func(t *testing.T) {
		targetCall := 2
		actualHookCalls := 1
		var invokeCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					invokeCalls = append(invokeCalls, actualHookCalls)
				case targetCall:
					invokeCalls = append(invokeCalls, actualHookCalls)
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				switch actualHookCalls {
				case 1:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							actualHookCalls++
							return ctx.Err()
						}
					}
				case targetCall:
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
			assert.Equal(t, []int{1}, invokeCalls)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Worker afterhook context canceled with skip failurehook testing", func(t *testing.T) {
		targetCall := 2
		actualHookCalls := 0
		var invokeCalls []int

		envelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				actualHookCalls++

				switch actualHookCalls {
				case 1:
					select {
					case <-time.After(300 * time.Millisecond):
						select {
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				case targetCall:
				}
				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				invokeCalls = append(invokeCalls, actualHookCalls)
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
			assert.Equal(t, 0, len(invokeCalls))
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
	suite.Setup(t)

	t.Run("Envelope rescheduling if error is in beforehook calling testing", func(t *testing.T) {
		errorCall := 1
		successCall := 2
		currentCall := 0
		var isCalledMarkCounter []int

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				currentCall++

				switch currentCall {
				case errorCall:
					return fmt.Errorf("error")
				case successCall:
					return nil
				}
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledMarkCounter = append(isCalledMarkCounter, currentCall)
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
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1*time.Second + 100*time.Millisecond):
				assert.Equal(t, []int{successCall}, isCalledMarkCounter)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope rescheduling if error is in invoke calling testing", func(t *testing.T) {
		errorCall := 1
		successCall := 2
		currentCall := 0
		invokeMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				currentCall++

				switch currentCall {
				case errorCall:
					return fmt.Errorf("error")
				case successCall:
					invokeMark = true
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
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1*time.Second + 100*time.Millisecond):
				assert.Equal(t, true, invokeMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope default interval calling if error is in afterhook testing", func(t *testing.T) {
		currentCall := 0
		var isCalledMarkCounter []int

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(1*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				currentCall++
				isCalledMarkCounter = append(isCalledMarkCounter, currentCall)

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
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)

			select {
			case <-time.After(1*time.Second + 100*time.Millisecond):
				assert.Equal(t, []int{1, 2}, isCalledMarkCounter)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}

func TestFailureHookDecisionsOnOneTimeEnvelopes(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

	t.Run("Envelope default decision if WithFailureHook is nil testing", func(t *testing.T) {
		targetCalls := 1
		currentCalls := 0
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch currentCalls {
				case 0:
					currentCalls++
					return fmt.Errorf("some error")
				case 1:
					currentCalls++
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return nil
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successMark = true
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
				assert.Equal(t, targetCalls, currentCalls)
				assert.Equal(t, false, successMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope WithFailureHook default decision testing", func(t *testing.T) {
		targetCalls := 1
		currentCalls := 0
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch currentCalls {
				case 0:
					currentCalls++
					return fmt.Errorf("some error")
				case 1:
					currentCalls++
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.DefaultOnceDecision()
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successMark = true
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
				assert.Equal(t, targetCalls, currentCalls)
				assert.Equal(t, false, successMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope WithFailureHook now timeout testing", func(t *testing.T) {
		targetRetry := 1
		currentRetry := 0
		isCalledInRetryCount := 0
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				time.Sleep(500 * time.Millisecond)
				switch currentRetry {
				case 0:
					currentRetry++
					return fmt.Errorf("some error")
				case targetRetry:
					isCalledInRetryCount++
					currentRetry++
				case targetRetry + 1:
					isCalledInRetryCount++
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceNowDecision()
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successMark = true
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

			// первый вызов внутри invoke с ошибкой
			select {
			case <-time.After(750 * time.Millisecond):
				assert.Equal(t, 0, isCalledInRetryCount)
				assert.Equal(t, false, successMark)
			}

			// второй вызов внутри invoke (уже успешный)
			select {
			case <-time.After(750 * time.Millisecond):
				assert.Equal(t, 1, isCalledInRetryCount)
				assert.Equal(t, true, successMark)
			}

			// нового вызова нет, после повторного вызова в invoke envelope забыт через Forget
			select {
			case <-time.After(750 * time.Millisecond):
				assert.Equal(t, 1, isCalledInRetryCount)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope WithFailureHook deadline timeout testing", func(t *testing.T) {
		targetRetry := 1
		currentRetry := 0
		isCalledInRetryCount := 0
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch currentRetry {
				case 0:
					currentRetry++
					return fmt.Errorf("some error")
				case targetRetry:
					isCalledInRetryCount++
					currentRetry++
				case targetRetry + 1:
					isCalledInRetryCount++
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(2 * time.Second)
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successMark = true
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

			// первый вызов invoke
			select {
			case <-time.After(500 * time.Millisecond):
				assert.Equal(t, 0, isCalledInRetryCount)
				assert.Equal(t, false, successMark)
			}

			// invoke отложен с дедлайном в WithFailureHook, isCalledInRetryCount тот же
			select {
			case <-time.After(500 * time.Millisecond):
				assert.Equal(t, 0, isCalledInRetryCount)
				assert.Equal(t, false, successMark)
			}

			// дедлайн из WithFailureHook прошел, новый вызов invoke
			select {
			case <-time.After(2 * time.Second):
				assert.Equal(t, 1, isCalledInRetryCount)
				assert.Equal(t, true, successMark)
			}

			// нового вызова нет, после повторного вызова в invoke envelope забыт через Forget
			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, 1, isCalledInRetryCount)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope set default deadline timeout if WithFailureHook timeout is 0 testing", func(t *testing.T) {
		targetRetry := 1
		currentRetry := 0
		isCalledInRetryCount := 0
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				switch currentRetry {
				case 0:
					currentRetry++
					return fmt.Errorf("some error")
				case targetRetry:
					isCalledInRetryCount++
					currentRetry++
				case targetRetry + 1:
					isCalledInRetryCount++
				}

				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.RetryOnceAfterDecision(0) // default = 30s
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successMark = true
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

			// первый вызов invoke
			select {
			case <-time.After(500 * time.Millisecond):
				assert.Equal(t, 0, isCalledInRetryCount)
				assert.Equal(t, false, successMark)
			}

			// invoke отложен с дедлайном в WithFailureHook, isCalledInRetryCount тот же
			select {
			case <-time.After(500 * time.Millisecond):
				assert.Equal(t, 0, isCalledInRetryCount)
				assert.Equal(t, false, successMark)
			}

			// дедлайн из WithFailureHook прошел, новый вызов invoke
			select {
			case <-time.After(30 * time.Second):
				assert.Equal(t, 1, isCalledInRetryCount)
				assert.Equal(t, true, successMark)
			}

			// нового вызова нет, после повторного вызова в invoke envelope забыт через Forget
			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, 1, isCalledInRetryCount)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Multiple envelope with different timeouts in WithFailureHook testing", func(t *testing.T) {
		targetCalls := []string{"first", "third", "fifth", "forth", "second"}
		var actualCalls []string

		firstEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCalls = append(actualCalls, "first")
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
					return fmt.Errorf("some error")
				case targetRetrySecond:
					actualCalls = append(actualCalls, "second")
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
				actualCalls = append(actualCalls, "third")
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
					return fmt.Errorf("some error")
				case targetRetryForth:
					actualCalls = append(actualCalls, "forth")
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
					return fmt.Errorf("some error")
				default:
					actualCalls = append(actualCalls, "fifth")
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
				assert.Equal(t, targetCalls, actualCalls)
			}
			fmt.Println(actualCalls)
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
	suite.Setup(t)

	t.Run("Envelope success hook testing", func(t *testing.T) {
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithScheduleModeInterval(0),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
				successMark = true
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
				assert.Equal(t, true, successMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope success hook is not called when beforehook has error testing", func(t *testing.T) {
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
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
				successMark = true
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
				assert.Equal(t, false, successMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope success hook is not called when invoke has error testing", func(t *testing.T) {
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
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
				successMark = true
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
				assert.Equal(t, false, successMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	t.Run("Envelope success hook ignores error in afterhook testing", func(t *testing.T) {
		successMark := false

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
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
				successMark = true
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
				assert.Equal(t, true, successMark)
			}
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}
