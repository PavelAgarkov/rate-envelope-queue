package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	req "github.com/simplegear/rate-envelope-queue"
)

type User struct {
	Name  string
	Email string
	Age   int
}

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

	user1 := &User{
		Name:  "John",
		Email: "@gmail.com",
		Age:   30,
	}
	emailEnvelope, err := req.NewEnvelope(
		req.WithId(1),
		req.WithType("email_1"),
		//WithInterval(6*time.Second),
		req.WithDeadline(5*time.Second),
		req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("hook before email 1 ", envelope.GetId(), time.Now())
			//return ErrStopTask
			return nil
		}),
		req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v1", time.Now())
			user := envelope.GetPayload().(*User)
			fmt.Println("user:", user.Name, user.Email, user.Age)
			user.Name = "Changed Name"
			envelope.UpdatePayload(user)
			return nil
		}),
		req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("hook after email 1 ", envelope.GetId(), time.Now())
			user := envelope.GetPayload().(*User)
			fmt.Println("user:", user.Name, user.Email, user.Age)
			// скипнет дальнейшую обработку конверта
			//return ErrStopEnvelope
			//return nil
			// любая ошибка, которая не ErrStopEnvelope будет обработана в failureHook
			return fmt.Errorf("some error after")
		}),
		req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
			fmt.Println("failure hook email 1", envelope.GetId(), time.Now(), err)
			//return DefaultOnceDecision()
			//return RetryOnceNowDecision()
			return req.RetryOnceAfterDecision(time.Second * 5)
			//return nil
		}),
		req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
			fmt.Println("success hook email 1", envelope.GetId(), time.Now())
		}),
		req.WithStampsPerEnvelope(req.LoggingStamp()),
		req.WithPayload(user1),
	)
	if err != nil {
		panic(err)
	}

	emailEnvelope1, err := req.NewEnvelope(
		req.WithId(1),
		req.WithType("email_2"),
		req.WithScheduleModeInterval(5*time.Second),
		req.WithDeadline(3*time.Second),
		req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
			time.Sleep(5 * time.Second)
			fmt.Println("📧 Email v2", time.Now())
			return nil
		}),
		req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("hook before email 2", envelope.GetId(), time.Now())
			//return ErrStopTask
			return nil
		}),
		req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("hook after email 2", envelope.GetId(), time.Now())
			return req.ErrStopEnvelope
		}),
	)
	if err != nil {
		panic(err)
	}

	metricsEnvelope, err := req.NewEnvelope(
		req.WithId(2),
		req.WithType("metrics"),
		req.WithScheduleModeInterval(3*time.Second),
		req.WithDeadline(1*time.Second),
		req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
		req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("📧 Metrics v2", time.Now())
			return nil
		}),
		req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
	)
	if err != nil {
		panic(err)
	}

	metricsEnvelope1, err := req.NewEnvelope(
		req.WithId(2),
		req.WithType("metrics_1"),
		req.WithScheduleModeInterval(0),
		req.WithDeadline(1*time.Second),
		req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
		req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("📧 Metrics v1", time.Now())
			return nil
		}),
		req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
	)
	if err != nil {
		panic(err)
	}

	metricsEnvelope3, err := req.NewEnvelope(
		req.WithId(2),
		req.WithType("metrics_3"),
		req.WithScheduleModeInterval(0),
		req.WithDeadline(1*time.Second),
		req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
		req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("📧 Metrics v3", time.Now())
			return errors.New("some error")
		}),
		req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
	)
	if err != nil {
		panic(err)
	}

	foodEnvelope, err := req.NewEnvelope(
		req.WithId(3),
		req.WithType("food"),
		req.WithScheduleModeInterval(2*time.Second),
		req.WithDeadline(1*time.Second),
		req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
		req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
			fmt.Println("📧Fooding v2", time.Now())
			return nil
		}),
		req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
			return nil
		}),
	)
	if err != nil {
		panic(err)
	}

	envelops := map[string]*req.Envelope{
		"Email":   emailEnvelope,
		"Metrics": metricsEnvelope,
		"Food":    foodEnvelope,
	}

	envelopeQueue := req.NewRateEnvelopeQueue(
		parent,
		"test_queue",
		req.WithLimitOption(5),
		req.WithWaitingOption(true),
		req.WithStopModeOption(req.Drain),
		req.WithAllowedCapacityOption(50),
	)

	start := func() {
		envelopeQueue.Start()
		err := envelopeQueue.Send(envelops["Email"], envelops["Metrics"], envelops["Food"])
		if err != nil {
			fmt.Println("add err:", err)
		}
		err = envelopeQueue.Send(emailEnvelope1)
		if err != nil {
			fmt.Println("add err:", err)
		}
		err = envelopeQueue.Send(emailEnvelope, emailEnvelope1, metricsEnvelope1, metricsEnvelope3)
		if err != nil {
			fmt.Println("add err:", err)
		}
	}
	stop := func() {
		envelopeQueue.Stop()
	}

	start()
	stop()
	err = envelopeQueue.Send(foodEnvelope)
	if err != nil {
		fmt.Println("add err after stop:", err)
	}
	start()

	go func() {
		for i := range 15 {
			user1 := &User{
				Name:  "John",
				Email: "@gmail.com",
				Age:   30,
			}
			id := i + 1
			emailEnvelope, err = req.NewEnvelope(
				req.WithId(uint64(id)),
				req.WithType("email_1"),
				//WithInterval(6*time.Second),
				req.WithDeadline(5*time.Second),
				req.WithBeforeHook(func(ctx context.Context, envelope *req.Envelope) error {
					fmt.Println("hook before email 1 ", envelope.GetId(), time.Now())
					//return ErrStopTask
					return nil
				}),
				req.WithInvoke(func(ctx context.Context, envelope *req.Envelope) error {
					time.Sleep(5 * time.Second)
					fmt.Println("📧 Email v1", time.Now())
					user := envelope.GetPayload().(*User)
					fmt.Println("user:", user.Name, user.Email, user.Age)
					user.Name = "Changed Name"
					envelope.UpdatePayload(user)
					return nil
				}),
				req.WithAfterHook(func(ctx context.Context, envelope *req.Envelope) error {
					fmt.Println("hook after email 1 ", envelope.GetId(), time.Now())
					user := envelope.GetPayload().(*User)
					fmt.Println("user:", user.Name, user.Email, user.Age)
					// скипнет дальнейшую обработку конверта
					//return ErrStopEnvelope
					//return nil
					// любая ошибка, которая не ErrStopEnvelope будет обработана в failureHook
					return fmt.Errorf("some error after")
				}),
				req.WithFailureHook(func(ctx context.Context, envelope *req.Envelope, err error) req.Decision {
					fmt.Println("failure hook email 1", envelope.GetId(), time.Now(), err)
					return req.DefaultOnceDecision()
					//return RetryOnceNowDecision()
					//return RetryOnceAfterDecision(time.Second * 5)
					//return nil
				}),
				req.WithSuccessHook(func(ctx context.Context, envelope *req.Envelope) {
					fmt.Println("success hook email 1", envelope.GetId(), time.Now())
				}),
				req.WithStampsPerEnvelope(req.LoggingStamp()),
				req.WithPayload(user1),
			)
			if err != nil {
				panic(err)
			}
			err = envelopeQueue.Send(emailEnvelope)
			if err != nil {
				fmt.Println("add err async:", err)
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
	stop()
	fmt.Println("queue: done")
}
