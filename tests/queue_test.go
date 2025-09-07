package tests

import (
	"context"
	"testing"
	"time"

	req "github.com/PavelAgarkov/rate-envelope-queue"
	"github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T) {
	suite := &TestSuite{}
	suite.Setup(t, 20*time.Second)

	t.Run("Different start and stop queue options", func(t *testing.T) {
		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
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
			envelopeQueue.Start()
			err = envelopeQueue.Send(someEnvelope)
			assert.NoError(t, err)
			envelopeQueue.Stop()

			envelopeQueue.Stop()
			envelopeQueue.Start()
			rerr := envelopeQueue.Send(someEnvelope)
			assert.NoError(t, rerr)
			envelopeQueue.Stop()

			envelopeQueue.Stop()
			envelopeQueue.Stop()
			serr := envelopeQueue.Send(someEnvelope)
			assert.Error(t, serr)
			assert.ErrorIs(t, serr, req.ErrEnvelopeQueueIsNotRunning)
		}

		test()
	})

	t.Run("Queue stopping with 'Stop' stop mode and waiting 'true' option", func(t *testing.T) {
		invokeMarkCh := make(chan bool, 2)

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- true
				}
				return nil
			}),
		)
		assert.NoError(t, err)

		someEnvelope2, err := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- true
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
			req.WithStopModeOption(req.Stop),
		)

		test := func() {
			err = envelopeQueue.Send(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, 2, len(invokeMarkCh))
			}
		}

		test()
	})

	t.Run("Queue stopping with 'Drain' stop mode and waiting 'true' option", func(t *testing.T) {
		invokeMarkCh := make(chan bool, 2)

		someEnvelope, err := req.NewEnvelope(
			req.WithId(1),
			req.WithType("envelope_1"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- true
				}
				return nil
			}),
		)
		assert.NoError(t, err)

		someEnvelope2, err := req.NewEnvelope(
			req.WithId(2),
			req.WithType("envelope_2"),
			req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
				select {
				case <-time.After(1 * time.Second):
					invokeMarkCh <- true
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

		test := func() {
			err = envelopeQueue.Send(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, 2, len(invokeMarkCh))
			}
		}

		test()
	})

	t.Run("Queue stopping with 'Drain' stop mode and waiting 'false' option", func(t *testing.T) {
		invokeMarkCh := make(chan struct{}, 2)

		someEnvelope, err := req.NewEnvelope(
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
		assert.NoError(t, err)

		someEnvelope2, err := req.NewEnvelope(
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
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(false),
			req.WithStopModeOption(req.Drain),
		)

		test := func() {
			err = envelopeQueue.Send(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, 0, len(invokeMarkCh))
			}
		}

		test()
	})

	t.Run("Queue stopping with 'Stop' stop mode and waiting 'false' option", func(t *testing.T) {
		invokeMarkCh := make(chan struct{}, 2)

		someEnvelope, err := req.NewEnvelope(
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
		assert.NoError(t, err)

		someEnvelope2, err := req.NewEnvelope(
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
		assert.NoError(t, err)

		envelopeQueue := req.NewRateEnvelopeQueue(
			suite.ctx,
			"queue",
			req.WithLimitOption(1),
			req.WithWaitingOption(false),
			req.WithStopModeOption(req.Stop),
		)

		test := func() {
			err = envelopeQueue.Send(someEnvelope, someEnvelope2)
			assert.NoError(t, err)

			envelopeQueue.Start()
			envelopeQueue.Stop()

			select {
			case <-time.After(3 * time.Second):
				assert.Equal(t, 0, len(invokeMarkCh))
			}
		}

		test()
	})
}
