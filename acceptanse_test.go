package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PavelAgarkov/rate-pool/pkg"
)

func Test_Acceptance(t *testing.T) {
	patent, cancel := context.WithCancel(context.Background())
	defer cancel()

	//logger := log.New(os.Stdout, "", log.LstdFlags)

	emailEnvelope := pkg.NewEnvelope(
		pkg.WithId(1),
		pkg.WithType("email"),
		pkg.WithInterval(5*time.Second),
		pkg.WithDeadline(3*time.Second),
		pkg.WithInvoke(func(ctx context.Context) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v2", time.Now())
			return nil
		}),
		pkg.WithBeforeHook(func(ctx context.Context, envelope *pkg.Envelope) error {
			fmt.Println("hook before email", envelope.GetId(), time.Now())
			//return pkg.ErrStopTask
			return nil
		}),
		pkg.WithAfterHook(func(ctx context.Context, envelope *pkg.Envelope) error {
			fmt.Println("hook after email", envelope.GetId(), time.Now())
			return pkg.ErrStopEnvelope
		}),
	)

	metricsEnvelope := pkg.NewEnvelope(
		pkg.WithId(2),
		pkg.WithType("metrics"),
		pkg.WithInterval(3*time.Second),
		pkg.WithDeadline(1*time.Second),
		pkg.WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧 Metrics", time.Now())
			return nil
		}),
		pkg.WithBeforeHook(func(ctx context.Context, envelope *pkg.Envelope) error {
			return nil
		}),
		pkg.WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧 Metrics v2", time.Now())
			return nil
		}),
		pkg.WithAfterHook(func(ctx context.Context, envelope *pkg.Envelope) error {
			return nil
		}),
	)

	foodEnvelope := pkg.NewEnvelope(
		pkg.WithId(3),
		pkg.WithType("food"),
		pkg.WithInterval(2*time.Second),
		pkg.WithDeadline(1*time.Second),
		pkg.WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧Fooding", time.Now())
			return nil
		}),
		pkg.WithBeforeHook(func(ctx context.Context, envelope *pkg.Envelope) error {
			return nil
		}),
		pkg.WithInvoke(func(ctx context.Context) error {
			fmt.Println("📧Fooding v2", time.Now())
			return nil
		}),
		pkg.WithAfterHook(func(ctx context.Context, envelope *pkg.Envelope) error {
			return nil
		}),
	)

	envelops := map[string]*pkg.Envelope{
		"Email":   emailEnvelope,
		"Metrics": metricsEnvelope,
		"Food":    foodEnvelope,
	}

	envelopeQueue := pkg.NewRateEnvelopeQueue(
		pkg.WithLimitOption(3),
		pkg.WithWaitingOption(true),
		pkg.WithStopModeOption(pkg.Drain),
		//pkg.WithStamps(
		//	pkg.LoggingStamp(logger),
		//	pkg.BeforeAfterStamp(pkg.WithHookTimeout),
		//),
	)

	envelopeQueue.Start(patent)
	err := envelopeQueue.Add(envelops["Email"], envelops["Metrics"], envelops["Food"], emailEnvelope)
	if err != nil {
		fmt.Println("add err:", err)
	}

	time.Sleep(1 * time.Second)
	envelopeQueue.Stop()

	envelopeQueue = pkg.NewRateEnvelopeQueue(
		pkg.WithLimitOption(3),
		pkg.WithWaitingOption(true),
		pkg.WithStopModeOption(pkg.Drain),
		pkg.WithStamps(
			pkg.BeforeAfterStamp(pkg.WithHookTimeout),
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
