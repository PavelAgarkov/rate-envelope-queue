package pkg

import "context"

type QueuePool interface {
	Start(ctx context.Context)
	Add(task ...*Envelope) error
	Stop()
}
