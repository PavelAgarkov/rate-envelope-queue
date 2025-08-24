package rate_envelope_queue

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func Test_Acceptance(t *testing.T) {
	patent, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	go func() {
		<-sig
		fmt.Println("signal shutdown")
		cancel()
	}()

	//logger := log.New(os.Stdout, "", log.LstdFlags)

	emailEnvelope := NewEnvelope(
		WithId(1),
		WithType("email"),
		WithInterval(5*time.Second),
		WithDeadline(3*time.Second),
		WithInvoke(func(ctx context.Context) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v2", time.Now())
			return nil
		}),
		WithBeforeHook(func(ctx context.Context, envelope *Envelope) error {
			fmt.Println("hook before email", envelope.GetId(), time.Now())
			//return ErrStopTask
			return nil
		}),
		WithAfterHook(func(ctx context.Context, envelope *Envelope) error {
			fmt.Println("hook after email", envelope.GetId(), time.Now())
			return ErrStopEnvelope
		}),
	)

	metricsEnvelope := NewEnvelope(
		WithId(2),
		WithType("metrics"),
		WithInterval(3*time.Second),
		WithDeadline(1*time.Second),
		WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧 Metrics", time.Now())
			return nil
		}),
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

	foodEnvelope := NewEnvelope(
		WithId(3),
		WithType("food"),
		WithInterval(2*time.Second),
		WithDeadline(1*time.Second),
		WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧Fooding", time.Now())
			return nil
		}),
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
		WithLimitOption(3),
		WithWaitingOption(true),
		WithStopModeOption(Drain),
		//WithStamps(
		//	LoggingStamp(logger),
		//	BeforeAfterStamp(WithHookTimeout),
		//),
	)

	envelopeQueue.Start(patent)
	err := envelopeQueue.Add(envelops["Email"], envelops["Metrics"], envelops["Food"], emailEnvelope)
	if err != nil {
		fmt.Println("add err:", err)
	}

	time.Sleep(1 * time.Second)
	envelopeQueue.Stop()

	envelopeQueue = NewRateEnvelopeQueue(
		WithLimitOption(3),
		WithWaitingOption(true),
		WithStopModeOption(Drain),
		WithStamps(
			BeforeAfterStamp(WithHookTimeout),
		),
	)

	envelopeQueue.Start(patent)
	err = envelopeQueue.Add(envelops["Email"], envelops["Metrics"], envelops["Food"])
	if err != nil {
		fmt.Println("add err:", err)
	}

	go func() {
		select {
		case <-time.After(25 * time.Second):
			fmt.Println("main: timeout")
			cancel()
		}
	}()

	<-patent.Done()
	fmt.Println("parent: done")
	envelopeQueue.Stop()
	fmt.Println("queue: done")
}
