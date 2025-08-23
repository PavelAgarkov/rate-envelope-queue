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

	emailTask := &pkg.PoolTask{Id: 1, Interval: 5 * time.Second, Deadline: 3 * time.Second, Type: "email",
		Call: func(ctx context.Context) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v1", time.Now())
			return nil
		}}
	tasks := map[string]*pkg.PoolTask{
		"Email": emailTask,
		"Metrics": {Id: 2, Interval: 3 * time.Second, Deadline: 1 * time.Second, Type: "metrics",
			Call: func(ctx context.Context) error {
				fmt.Println("📧 Metrics", time.Now())
				return nil
			}},
		"Food": {Id: 3, Interval: 2 * time.Second, Deadline: 1 * time.Second, Type: "food",
			Call: func(ctx context.Context) error {
				fmt.Println("📧Fooding", time.Now())
				return nil
			}},
	}
	pool := pkg.NewPool(
		pkg.WithLimitOption(3),
		pkg.WithWaitingOption(true),
		pkg.WithStopModeOption(pkg.Drain),
		pkg.WithLimiterOption(nil),
		pkg.WithWorkqueueConfigOption(nil),
	)

	pool.Start(patent)
	err := pool.Add(tasks["Email"], tasks["Metrics"], tasks["Food"], emailTask)
	if err != nil {
		fmt.Println("add err:", err)
	}

	time.Sleep(1 * time.Second)
	pool.Stop()

	pool = pkg.NewPool(
		pkg.WithLimitOption(3),
		pkg.WithWaitingOption(true),
		pkg.WithStopModeOption(pkg.Drain),
		pkg.WithLimiterOption(nil),
		pkg.WithWorkqueueConfigOption(nil),
	)

	pool.Start(patent)
	err = pool.Add(tasks["Email"], tasks["Metrics"], tasks["Food"])
	if err != nil {
		fmt.Println("add err:", err)
	}

	go func() {
		select {
		case <-time.After(15 * time.Second):
			fmt.Println("main: timeout")
			cancel()
		}
	}()

	<-patent.Done()
	fmt.Println("parent: done")
	pool.Stop()
	//time.Sleep(5 * time.Second)
	fmt.Println("queue: done")
}
