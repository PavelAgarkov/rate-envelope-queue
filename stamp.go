package rate_envelope_queue

import (
	"context"
	"log"
	"time"
)

func LoggingStamp() Stamp {
	return func(next Invoker) Invoker {
		return func(ctx context.Context, envelope *Envelope) error {
			t0 := time.Now()
			err := next(ctx, envelope)
			log.Printf("%s %s/%d dur=%s err=%v", service, envelope._type, envelope.id, time.Since(t0), err)
			return err
		}
	}
}
