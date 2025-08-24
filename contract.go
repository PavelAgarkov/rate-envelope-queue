package rate_envelope_queue

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrStopEnvelope                        = errors.New(fmt.Sprintf("%s: stop envelope", service))
	ErrEnvelopeInBlacklist                 = errors.New(fmt.Sprintf("%s: envelope is in blacklist", service))
	ErrEnvelopeQueueIsNotRunning           = errors.New(fmt.Sprintf("%s: queue is not running", service))
	ErrAdditionEnvelopeToQueueBadFields    = errors.New(fmt.Sprintf("%s: addition envelope to queue has bad fields", service))
	ErrAdditionEnvelopeToQueueBadIntervals = errors.New(fmt.Sprintf("%s: addition envelope to queue has bad intervals", service))
)

type (
	StopMode string
	Invoker  func(ctx context.Context, envelope *Envelope) error
	Stamp    func(next Invoker) Invoker
)

const (
	Drain StopMode = "drain"
	Stop  StopMode = "stop"

	service = "[rate-envelope-queue]"
)

type QueuePool interface {
	Start(ctx context.Context)
	Add(envelopes ...*Envelope) error
	Stop()
}
