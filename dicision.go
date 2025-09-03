package rate_envelope_queue

import "time"

type destinationState string

const (
	DestinationStateField                    = "type"
	PayloadAfterField                        = "after"
	DecisionStateRetryNow   destinationState = "retry_now"
	DecisionStateRetryAfter destinationState = "retry_after"
	DecisionStateDrop       destinationState = "drop"
)

type Payload map[string]interface{}

type Decision interface {
	Payload() Payload
}

func DefaultOnceDecision() Decision {
	return &DefaultDecision{
		data: map[string]interface{}{
			DestinationStateField: DecisionStateDrop,
		},
	}
}

type DefaultDecision struct {
	data map[string]interface{}
}

func (d *DefaultDecision) Payload() Payload {
	return d.data
}

func RetryOnceNowDecision() Decision {
	return &RetryOnceDecision{
		data: map[string]interface{}{
			DestinationStateField: DecisionStateRetryNow,
		},
	}
}

func RetryOnceAfterDecision(after time.Duration) Decision {
	if after <= 0 {
		after = time.Second
	}
	return &RetryOnceDecision{
		data: map[string]interface{}{
			DestinationStateField: DecisionStateRetryAfter,
			PayloadAfterField:     after,
		},
	}
}

type RetryOnceDecision struct {
	data map[string]interface{}
}

func (d *RetryOnceDecision) Payload() Payload {
	return d.data
}
