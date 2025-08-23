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

	emailEnvelope := &pkg.Envelope{Id: 1, Interval: 5 * time.Second, Deadline: 3 * time.Second, Type: "email",
		Invoke: func(ctx context.Context) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v1", time.Now())
			return nil
		},
		BeforeHook: func(ctx context.Context, item *pkg.Envelope) error {
			fmt.Println("hook before email", item.Id, time.Now())
			//return pkg.ErrStopTask
			return nil
		},
		AfterHook: func(ctx context.Context, item *pkg.Envelope) error {
			fmt.Println("hook after email", item.Id, time.Now())
			return pkg.ErrStopEnvelope
		},
	}
	envelops := map[string]*pkg.Envelope{
		"Email": emailEnvelope,
		"Metrics": {Id: 2, Interval: 3 * time.Second, Deadline: 1 * time.Second, Type: "metrics",
			Invoke: func(ctx context.Context) error {
				fmt.Println("📧 Metrics", time.Now())
				return nil
			}},
		"Food": {Id: 3, Interval: 2 * time.Second, Deadline: 1 * time.Second, Type: "food",
			Invoke: func(ctx context.Context) error {
				fmt.Println("📧Fooding", time.Now())
				return nil
			}},
	}
	envelopeQueue := pkg.NewRateEnvelopeQueue(
		pkg.WithLimitOption(3),
		pkg.WithWaitingOption(true),
		pkg.WithStopModeOption(pkg.Drain),
		pkg.WithLimiterOption(nil),
		pkg.WithWorkqueueConfigOption(nil),
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
		pkg.WithLimiterOption(nil),
		pkg.WithWorkqueueConfigOption(nil),
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
	//time.Sleep(5 * time.Second)
	fmt.Println("queue: done")
}
