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
		ticker := time.NewTicker(1 * time.Second)
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
