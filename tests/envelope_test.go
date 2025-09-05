package tests

import (
	"context"
	"fmt"
	req "github.com/PavelAgarkov/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestEnvelope(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

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
				time.Sleep(deadline * 2) // т.к deadline умножается на frac=0.5
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					beforeHookIdGetter = envelope.GetId()
					return nil
				}
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				time.Sleep(deadline * 2) // т.к deadline умножается на frac=0.5
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
		envelopeDeadline := 1 * time.Second
		inHookDeadline := 900 * time.Millisecond // не сработает, т.к min deadline = 800ms

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
				case <-time.After(inHookDeadline): // не сработает, т.к min deadline = 800ms
					beforeHookIdGetter = envelope.GetId()
					return nil
				}
			}),
			req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(inHookDeadline): // не сработает, т.к min deadline = 800ms
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

	// проверить NewDefaultDestination = DestinationStateDrop -> тогда просто forget
	t.Run("Envelope WithFailureHook default timeout testing", func(t *testing.T) {
		targetCalls := 1
		currentCalls := 0

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
			}

			// второй вызов внутри invoke (уже успешный)
			select {
			case <-time.After(750 * time.Millisecond):
				assert.Equal(t, 1, isCalledInRetryCount)
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
			}

			// invoke отложен с дедлайном в WithFailureHook, isCalledInRetryCount тот же
			select {
			case <-time.After(500 * time.Millisecond):
				assert.Equal(t, 0, isCalledInRetryCount)
			}

			// дедлайн из WithFailureHook прошел, новый вызов invoke
			select {
			case <-time.After(2 * time.Second):
				assert.Equal(t, 1, isCalledInRetryCount)
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
