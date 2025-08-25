package rate_envelope_queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func Test_Acceptance(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	go func() {
		<-sig
		fmt.Println("signal shutdown")
		cancel()
	}()

	emailEnvelope := NewEnvelope(
		WithId(1),
		WithType("email_1"),
		WithInterval(5*time.Second),
		WithDeadline(3*time.Second),
		WithInvoke(func(ctx context.Context) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v1", time.Now())
			return nil
		}),
		WithBeforeHook(func(ctx context.Context, envelope *Envelope) error {
			fmt.Println("hook before email 1 ", envelope.GetId(), time.Now())
			//return ErrStopTask
			return nil
		}),
		WithAfterHook(func(ctx context.Context, envelope *Envelope) error {
			fmt.Println("hook after email 1 ", envelope.GetId(), time.Now())
			return ErrStopEnvelope
		}),
	)

	emailEnvelope1 := NewEnvelope(
		WithId(1),
		WithType("email_2"),
		WithInterval(5*time.Second),
		WithDeadline(3*time.Second),
		WithInvoke(func(ctx context.Context) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v2", time.Now())
			return nil
		}),
		WithBeforeHook(func(ctx context.Context, envelope *Envelope) error {
			fmt.Println("hook before email 2", envelope.GetId(), time.Now())
			//return ErrStopTask
			return nil
		}),
		WithAfterHook(func(ctx context.Context, envelope *Envelope) error {
			fmt.Println("hook after email 2", envelope.GetId(), time.Now())
			return ErrStopEnvelope
		}),
	)

	metricsEnvelope := NewEnvelope(
		WithId(2),
		WithType("metrics"),
		WithInterval(3*time.Second),
		WithDeadline(1*time.Second),
		WithBeforeHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
		WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧 Metrics v2", time.Now())
			return nil
		}),
		WithAfterHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
	)

	metricsEnvelope1 := NewEnvelope(
		WithId(2),
		WithType("metrics_1"),
		WithInterval(0),
		WithDeadline(1*time.Second),
		WithBeforeHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
		WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧 Metrics v1", time.Now())
			return nil
		}),
		WithAfterHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
	)

	metricsEnvelope3 := NewEnvelope(
		WithId(2),
		WithType("metrics_3"),
		WithInterval(0),
		WithDeadline(1*time.Second),
		WithBeforeHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
		WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧 Metrics v3", time.Now())
			return errors.New("some error")
		}),
		WithAfterHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
	)

	foodEnvelope := NewEnvelope(
		WithId(3),
		WithType("food"),
		WithInterval(2*time.Second),
		WithDeadline(1*time.Second),
		WithBeforeHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
		WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧Fooding v2", time.Now())
			return nil
		}),
		WithAfterHook(func(ctx context.Context, envelope *Envelope) error {
			return nil
		}),
	)

	envelops := map[string]*Envelope{
		"Email":   emailEnvelope,
		"Metrics": metricsEnvelope,
		"Food":    foodEnvelope,
	}

	envelopeQueue := NewRateEnvelopeQueue(
		parent,
		WithLimitOption(5),
		WithWaitingOption(true),
		WithStopModeOption(Drain),
		WithStamps(
			BeforeAfterStamp(WithHookTimeout),
		),
	)

	start := func() {
		envelopeQueue.Start()
		err := envelopeQueue.Add(envelops["Email"], envelops["Metrics"], envelops["Food"], emailEnvelope, emailEnvelope1, metricsEnvelope1, metricsEnvelope3)
		if err != nil {
			fmt.Println("add err:", err)
		}
	}
	stop := func() {
		envelopeQueue.Stop()
	}

	start()
	stop()
	start()

	go func() {
		select {
		case <-time.After(25 * time.Second):
			fmt.Println("main: timeout")
			cancel()
		}
	}()

	<-parent.Done()
	fmt.Println("parent: done")
	stop()
	fmt.Println("queue: done")
}
