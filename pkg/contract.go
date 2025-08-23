package pkg

import "context"

type Pool interface {
	Start(ctx context.Context)
	Add(task ...*PoolTask) error
	Stop()
}
