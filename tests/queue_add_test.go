package tests

import (
	"context"
	"fmt"
	req "github.com/PavelAgarkov/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"testing"
	"time"
)

type TestSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (ts *TestSuite) Setup(t *testing.T) {
	ts.ctx, ts.cancel = context.WithTimeout(context.Background(), 1*time.Minute) // TODO
	t.Cleanup(ts.cancel)
}

func TestEnvelopeAddingToQueue(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

	t.Run("Single envelope adding to queue", func(t *testing.T) {
		isCalledInInvokeBeforeStart := false
		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvokeBeforeStart = true
				return nil
			}),
		)

		isCalledInInvokeAfterStart := false
		someEnvelope2 := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				isCalledInInvokeAfterStart = true
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			err := envelopeQueue.Add(someEnvelope2)
			assert.NoError(t, err)
			envelopeQueue.Start()

			select {
			case <-time.After(1 * time.Second):
				assert.Equal(t, true, isCalledInInvokeAfterStart)
			}

			envelopeQueue.Stop()

			envelopeQueue.Start()
			err = envelopeQueue.Add(someEnvelope)
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

		firstEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCallsOrder = append(actualCallsOrder, "first")
				return nil
			}),
		)
		secondEnvelope := req.NewEnvelope(
			req.WithId(2),
			req.WithType("secondEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCallsOrder = append(actualCallsOrder, "second")
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(firstEnvelope, secondEnvelope)
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
		invalidType := ""
		invalidInterval := -1
		invalidDeadline := -1
		var nilInvoke func(ctx context.Context, envelope *req.Envelope) error

		invalidTypeEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType(invalidType),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)

		nilInvokeEnvelope := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithInvoke(nilInvoke),
		)

		invalidIntervalEnvelope := req.NewEnvelope(
			req.WithId(3),
			req.WithType("envelope_3"),
			req.WithInterval(time.Duration(invalidInterval)),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)

		invalidDeadlineEnvelope := req.NewEnvelope(
			req.WithId(4),
			req.WithType("envelope_4"),
			req.WithInterval(0),
			req.WithDeadline(time.Duration(invalidDeadline)),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()

			err := envelopeQueue.Add(invalidTypeEnvelope)
			assert.Equal(t, req.ErrAdditionEnvelopeToQueueBadFields, err)

			err = envelopeQueue.Add(nilInvokeEnvelope)
			assert.Equal(t, req.ErrAdditionEnvelopeToQueueBadFields, err)

			err = envelopeQueue.Add(invalidIntervalEnvelope)
			assert.Equal(t, req.ErrAdditionEnvelopeToQueueBadFields, err)

			err = envelopeQueue.Add(invalidDeadlineEnvelope)
			assert.Equal(t, req.ErrAdditionEnvelopeToQueueBadFields, err)

		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	// negative
	t.Run("Adding to queue with not running queue", func(t *testing.T) {
		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			envelopeQueue.Stop()

			err := envelopeQueue.Add(someEnvelope)
			assert.ErrorIs(t, req.ErrEnvelopeQueueIsNotRunning, err)
		}

		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	// negative
	t.Run("Envelope adding to queue with mismatch timeouts", func(t *testing.T) {
		firstEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithInterval(1*time.Second),
			req.WithDeadline(3*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(firstEnvelope)
			assert.ErrorIs(t, req.ErrAdditionEnvelopeToQueueBadIntervals, err)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})

	// negative
	t.Run("Envelope adding to queue with empty invoke", func(t *testing.T) {
		firstEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithInterval(2*time.Second),
			req.WithDeadline(1*time.Second),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(firstEnvelope)
			assert.ErrorIs(t, req.ErrAdditionEnvelopeToQueueBadFields, err)
		}
		stop := func() {
			envelopeQueue.Stop()
		}

		start()
		stop()
	})
}

func TestEnvelope(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

	t.Run("Envelope base interface testing", func(t *testing.T) {
		l := log.New(os.Stdout, "", 0)
		stamps := []req.Stamp{req.LoggingStamp(l)}

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
			req.WithStampsPerEnvelope(
				stamps...,
			),
		)

		assert.Equal(t, someEnvelope.GetId(), uint64(1))
		assert.Equal(t, someEnvelope.GetType(), "someEnvelope")
		assert.Equal(t, len(someEnvelope.GetStamps()), len(stamps))
	})

	t.Run("Envelope interval calling testing", func(t *testing.T) {
		invokeCounter := make(chan bool, 2)
		defer close(invokeCounter)

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(2*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				invokeCounter <- true
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
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

	t.Run("Envelope interval calling testing with time greater then interval", func(t *testing.T) {
		invokeCounter := make(chan bool, 2)
		defer close(invokeCounter)

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(2*time.Second),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(3 * time.Second): // не слушаем контекст дедлайна
					invokeCounter <- true
				}

				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
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

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(0),
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

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
			assert.NoError(t, err)

			ticker := time.NewTicker(3 * time.Second)
			select {
			case <-ticker.C:
				assert.Equal(t, false, invokeMark)
				ticker.Stop()
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

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(0),
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

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
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

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(0),
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

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
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

	// проверить NewDefaultDestination = DestinationStateDrop
	t.Run("Envelope WithFailureHook default timeout testing", func(t *testing.T) {
		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return fmt.Errorf("some error")
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.NewDefaultDestination()
				//return NewRetryNowDestination()
				//return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
			assert.NoError(t, err)

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

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(0),
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
				return req.NewRetryNowDestination()
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
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

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("someEnvelope"),
			req.WithInterval(0),
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
			// при таком кейсе WithFailureHook не отработает
			//req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
			//	return nil
			//}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.NewRetryAfterDestination(2 * time.Second)
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
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

		firstEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("firstEnvelope"),
			req.WithInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCalls = append(actualCalls, "first")
				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.NewRetryAfterDestination(1 * time.Second)
			}),
		)

		currentRetrySecond := 0
		targetRetrySecond := 1
		secondEnvelope := req.NewEnvelope(
			req.WithId(2),
			req.WithType("secondEnvelope"),
			req.WithInterval(0),
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
				return req.NewRetryAfterDestination(3 * time.Second)
			}),
		)

		thirdEnvelope := req.NewEnvelope(
			req.WithId(3),
			req.WithType("thirdEnvelope"),
			req.WithInterval(0),
			req.WithDeadline(1*time.Second),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				actualCalls = append(actualCalls, "third")
				return nil
			}),
			req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
				return req.NewRetryAfterDestination(1 * time.Second)
			}),
		)

		currentRetryForth := 0
		targetRetryForth := 1
		forthEnvelope := req.NewEnvelope(
			req.WithId(4),
			req.WithType("forthEnvelope"),
			req.WithInterval(0),
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
				return req.NewRetryAfterDestination(2 * time.Second)
			}),
		)

		failureCallFifth := 1
		fifthEnvelope := req.NewEnvelope(
			req.WithId(5),
			req.WithType("fifthEnvelope"),
			req.WithInterval(0),
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
				return req.NewRetryNowDestination()
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		start := func() {
			envelopeQueue.Start()
			err := envelopeQueue.Add(firstEnvelope, secondEnvelope, thirdEnvelope, forthEnvelope, fifthEnvelope)
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

func TestQueue(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t)

	t.Run("Different start and stop queue options", func(t *testing.T) {
		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		test := func() {
			envelopeQueue.Start()
			envelopeQueue.Start()
			err := envelopeQueue.Add(someEnvelope)
			assert.NoError(t, err)
			envelopeQueue.Stop()

			envelopeQueue.Stop()
			envelopeQueue.Start()
			rerr := envelopeQueue.Add(someEnvelope)
			assert.NoError(t, rerr)
			envelopeQueue.Stop()

			envelopeQueue.Stop()
			envelopeQueue.Stop()
			serr := envelopeQueue.Add(someEnvelope)
			assert.Equal(t, req.ErrEnvelopeQueueIsNotRunning, serr)
		}

		test()
	})

	t.Run("Queue stopping with 'Stop' stop mode and waiting 'true' option", func(t *testing.T) {
		invokeMarkCh := make(chan struct{}, 2)

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		someEnvelope2 := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Stop),
		)

		test := func() {
			err := envelopeQueue.Add(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(2 * time.Second):
				assert.Equal(t, 2, len(invokeMarkCh))
			}
		}

		test()
	})

	t.Run("Queue stopping with 'Drain' stop mode and waiting 'true' option", func(t *testing.T) {
		invokeMarkCh := make(chan struct{}, 2)

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		someEnvelope2 := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(true),
			req.WithStopModeOption(req.Drain),
		)

		test := func() {
			err := envelopeQueue.Add(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(2 * time.Second):
				assert.Equal(t, 2, len(invokeMarkCh))
			}
		}

		test()
	})

	t.Run("Queue stopping with 'Drain' stop mode and waiting 'false' option", func(t *testing.T) {
		invokeMarkCh := make(chan struct{}, 2)

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		someEnvelope2 := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(false),
			req.WithStopModeOption(req.Drain),
		)

		test := func() {
			err := envelopeQueue.Add(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, 1, len(invokeMarkCh))
			}
		}

		test()
	})

	t.Run("Queue stopping with 'Stop' stop mode and waiting 'false' option", func(t *testing.T) {
		invokeMarkCh := make(chan struct{}, 2)

		someEnvelope := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		someEnvelope2 := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- struct{}{}
				}
				return nil
			}),
		)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			req.WithLimitOption(1),
			req.WithWaitingOption(false),
			req.WithStopModeOption(req.Stop),
		)

		test := func() {
			err := envelopeQueue.Add(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, 1, len(invokeMarkCh))
			}
		}

		test()
	})
}
