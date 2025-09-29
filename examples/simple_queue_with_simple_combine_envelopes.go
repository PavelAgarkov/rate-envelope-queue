package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	req "github.com/simplegear/rate-envelope-queue"
)

func main() {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	go func() {
		<-sig
		fmt.Println("signal shutdown")
		cancel()
	}()

	queue := req.NewSimpleDrainQueue(
		parent,
		"some_queue",
		5,
	)

	queue.Start()

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-parent.Done():
				fmt.Println("sender: done")
				return
			case <-ticker.C:
				envelope, err := req.NewDynamicEnvelope(
					1*time.Second,
					func(ctx context.Context, envelope *req.Envelope) error {
						i++
						fmt.Println("📧 Email v1", i, time.Now())
						return nil
					},
					nil,
				)
				if err != nil {
					fmt.Println("main: can't create envelope", err)
					continue
				}
				err = queue.Send(envelope)
				if err != nil {
					fmt.Println("main: can't send envelope to queue", err)
					continue
				}
			}
		}
	}()

	go func() {
		envelope1, err := req.NewScheduleEnvelope(
			2*time.Second,
			1*time.Second,
			func(ctx context.Context, envelope *req.Envelope) error {
				fmt.Println("📧 Email v1", time.Now())
				return nil
			},
			nil,
		)
		if err != nil {
			fmt.Println("main: can't create envelope1", err)
			return
		}
		err = queue.Send(envelope1)
		if err != nil {
			fmt.Println("main: can't send envelope1 to queue", err)
			return
		}

		envelope2, err := req.NewScheduleEnvelope(
			4*time.Second,
			3*time.Second,
			func(ctx context.Context, envelope *req.Envelope) error {
				fmt.Println("📧 Email v2", time.Now())
				return nil
			},
			nil,
		)
		if err != nil {
			fmt.Println("main: can't create envelope2", err)
			return
		}
		err = queue.Send(envelope2)
		if err != nil {
			fmt.Println("main: can't send envelope2 to queue", err)
			return
		}

		envelope3, err := req.NewScheduleEnvelope(
			6*time.Second,
			2*time.Second,
			func(ctx context.Context, envelope *req.Envelope) error {
				fmt.Println("📧 Email v3", time.Now())
				return nil
			},
			nil,
		)
		if err != nil {
			fmt.Println("main: can't create envelope3", err)
			return
		}
		err = queue.Send(envelope3)
		if err != nil {
			fmt.Println("main: can't send envelope3 to queue", err)
			return
		}

	}()

	go func() {
		select {
		case <-time.After(25 * time.Second):
			fmt.Println("main: timeout")
			cancel()
		}
	}()

	<-parent.Done()
	fmt.Println("parent: done")
	queue.Stop()
	fmt.Println("queue: done")
}
